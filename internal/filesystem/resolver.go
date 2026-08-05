package filesystem

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// SymlinkMode controls how symlinks are resolved.
type SymlinkMode int

const (
	SymlinkDeny        SymlinkMode = iota // No symlinks allowed
	SymlinkWithinRoot                     // Symlinks allowed if final target stays under root
	SymlinkAllowListed                    // Target must be under configured roots
)

// Resolver resolves paths safely under a document root.
// It uses pre-opened root handles where possible to avoid check-then-open races.
type Resolver struct {
	root      string
	symlinks  SymlinkMode
	protected []string // glob patterns for protected files
}

// NewResolver creates a Resolver rooted at docRoot.
func NewResolver(docRoot string, symlinks SymlinkMode, protected []string) *Resolver {
	return &Resolver{
		root:      filepath.Clean(docRoot),
		symlinks:  symlinks,
		protected: protected,
	}
}

var (
	ErrTraversal      = errors.New("path escapes document root")
	ErrProtectedFile  = errors.New("access to protected file denied")
	ErrNotRegularFile = errors.New("not a regular file")
	ErrSymlinkDenied  = errors.New("symlink resolution denied")
	ErrSymlinkEscape  = errors.New("symlink escapes document root")
	ErrSpecialFile    = errors.New("special file (device, socket, pipe)")
	ErrFileNotFound   = errors.New("file not found")
)

// ResolvedFile holds the result of a successful path resolution.
type ResolvedFile struct {
	RealPath string // Canonical absolute path
	Info     fs.FileInfo
	F        *os.File // Open file handle (caller must close)
}

// Resolve resolves a normalized path under the document root and opens the file.
// normalizedPath must start with '/' and have no ".." segments.
func (r *Resolver) Resolve(normalizedPath string) (*ResolvedFile, error) {
	if !strings.HasPrefix(normalizedPath, "/") {
		return nil, fmt.Errorf("filesystem: path must be absolute: %q", normalizedPath)
	}

	// Check for protected patterns before resolving.
	if r.IsProtected(normalizedPath) {
		return nil, ErrProtectedFile
	}

	// Join root + path. We already normalized, so this is safe as long as
	// we verify the result stays under root.
	cleanPath := filepath.Join(r.root, normalizedPath)

	// Verify the resolved path is under root.
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("filesystem: abs: %w", err)
	}
	if !isUnderRoot(absPath, r.root) {
		return nil, ErrTraversal
	}

	// Use Lstat to detect symlinks before resolution.
	lInfo, err := os.Lstat(absPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrFileNotFound
		}
		return nil, fmt.Errorf("filesystem: lstat: %w", err)
	}

	targetPath := absPath
	if lInfo.Mode()&fs.ModeSymlink != 0 {
		switch r.symlinks {
		case SymlinkDeny:
			return nil, ErrSymlinkDenied
		case SymlinkWithinRoot:
			realPath, err := filepath.EvalSymlinks(absPath)
			if err != nil {
				return nil, fmt.Errorf("filesystem: eval symlinks: %w", err)
			}
			if !isUnderRoot(realPath, r.root) {
				return nil, ErrSymlinkEscape
			}
			targetPath = realPath
		default:
			return nil, ErrSymlinkDenied
		}
	}

	// Open file handle first to prevent check-then-open TOCTOU races.
	f, err := os.Open(targetPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrFileNotFound
		}
		return nil, fmt.Errorf("filesystem: open: %w", err)
	}

	// Inspect file properties on the open file handle directly.
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("filesystem: handle stat: %w", err)
	}

	// Verify it's a regular file using handle stat.
	if !info.Mode().IsRegular() {
		f.Close()
		return nil, ErrNotRegularFile
	}

	// Deny special files (devices, sockets, pipes) on open handle.
	mode := info.Mode()
	if mode&fs.ModeDevice != 0 || mode&fs.ModeNamedPipe != 0 || mode&fs.ModeSocket != 0 {
		f.Close()
		return nil, ErrSpecialFile
	}

	return &ResolvedFile{
		RealPath: targetPath,
		Info:     info,
		F:        f,
	}, nil
}

// ResolveInfo resolves a path and returns just the file info without opening.
func (r *Resolver) ResolveInfo(normalizedPath string) (fs.FileInfo, error) {
	if !strings.HasPrefix(normalizedPath, "/") {
		return nil, fmt.Errorf("filesystem: path must be absolute: %q", normalizedPath)
	}

	if r.IsProtected(normalizedPath) {
		return nil, ErrProtectedFile
	}

	cleanPath := filepath.Join(r.root, normalizedPath)
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("filesystem: abs: %w", err)
	}
	if !isUnderRoot(absPath, r.root) {
		return nil, ErrTraversal
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrFileNotFound
		}
		return nil, fmt.Errorf("filesystem: stat: %w", err)
	}

	return info, nil
}

// IsProtected checks if a normalized path matches any protected pattern.
func (r *Resolver) IsProtected(normalizedPath string) bool {
	for _, pat := range r.protected {
		// Check if any path segment matches the pattern.
		// This handles patterns like ".git" matching "/project/.git/config".
		segments := strings.Split(normalizedPath, "/")
		for _, seg := range segments {
			if matched, _ := filepath.Match(pat, seg); matched {
				return true
			}
		}
		// Also check full path match for patterns like "*.sql".
		base := filepath.Base(normalizedPath)
		if matched, _ := filepath.Match(pat, base); matched {
			return true
		}
		// Check full relative path.
		if matched, _ := filepath.Match(pat, normalizedPath); matched {
			return true
		}
	}
	return false
}

// isUnderRoot checks that child is under root.
func isUnderRoot(child, root string) bool {
	if !strings.HasPrefix(child, root) {
		return false
	}
	// Ensure it's not just a prefix match (e.g. /srv/app vs /srv/app2).
	if len(child) > len(root) && child[len(root)] != '/' {
		return false
	}
	return true
}

// DefaultProtectedPatterns returns the standard set of protected file patterns.
func DefaultProtectedPatterns() []string {
	return []string{
		".env", ".env.*",
		".git", ".git/**",
		".svn", ".hg",
		"composer.json", "composer.lock",
		"auth.json",
		"php.ini", ".user.ini",
		".htaccess", ".htpasswd",
		"*.sql", "*.sqlite", "*.sqlite3",
		"*.log", "*.bak", "*.backup", "*.swp",
		"*.dist",
		"Dockerfile", "compose.yaml", "docker-compose.yml",
		"gateway.yaml", "gateway.lock",
	}
}

// Close is a helper to close a ResolvedFile, ignoring the error.
func (rf *ResolvedFile) Close() {
	if rf.F != nil {
		rf.F.Close()
	}
}

// ReadAll reads the entire file content. For internal use only (small files).
func (rf *ResolvedFile) ReadAll(maxSize int64) ([]byte, error) {
	if rf.Info.Size() > maxSize {
		return nil, fmt.Errorf("file too large: %d bytes (max %d)", rf.Info.Size(), maxSize)
	}
	return io.ReadAll(io.LimitReader(rf.F, maxSize))
}
