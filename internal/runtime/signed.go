package runtime

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// SignedIndex represents a cryptographically signed runtime index.
type SignedIndex struct {
	Index     *Index `json:"index"`
	Signature string `json:"signature"` // hex-encoded Ed25519 signature
	KeyID     string `json:"key_id"`    // identifies the signing key
	Timestamp string `json:"timestamp"`
}

// IndexFetcher fetches and verifies signed runtime indexes.
type IndexFetcher struct {
	PublicKey ed25519.PublicKey
	Client    *http.Client
	CacheDir  string
}

// NewIndexFetcher creates an index fetcher with a trusted public key.
func NewIndexFetcher(pubKey ed25519.PublicKey, cacheDir string) *IndexFetcher {
	return &IndexFetcher{
		PublicKey: pubKey,
		Client:    &http.Client{Timeout: 30 * time.Second},
		CacheDir:  cacheDir,
	}
}

const maxSignedIndexSize = 10 * 1024 * 1024 // 10MB limit for signed index payload

// FetchAndVerify downloads a signed index, verifies the signature, and returns it.
func (f *IndexFetcher) FetchAndVerify(url string) (*Index, error) {
	resp, err := f.Client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch index: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch index: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSignedIndexSize))
	if err != nil {
		return nil, fmt.Errorf("read index body: %w", err)
	}

	// Parse signed envelope.
	var signed SignedIndex
	if err := json.Unmarshal(body, &signed); err != nil {
		return nil, fmt.Errorf("parse signed index: %w", err)
	}

	// Verify timestamp freshness (within 24 hours).
	ts, err := time.Parse(time.RFC3339, signed.Timestamp)
	if err != nil {
		return nil, fmt.Errorf("parse timestamp: %w", err)
	}
	if time.Since(ts) > 24*time.Hour {
		return nil, fmt.Errorf("index expired (signed at %s)", ts.Format(time.RFC3339))
	}

	// Reconstruct the signed payload.
	indexJSON, err := json.Marshal(signed.Index)
	if err != nil {
		return nil, fmt.Errorf("marshal index for verification: %w", err)
	}

	// The signed payload = index JSON + timestamp.
	payload := append(indexJSON, []byte(signed.Timestamp)...)

	// Decode signature.
	sigBytes, err := hex.DecodeString(signed.Signature)
	if err != nil {
		return nil, fmt.Errorf("decode signature: %w", err)
	}

	// Verify Ed25519 signature.
	if !ed25519.Verify(f.PublicKey, payload, sigBytes) {
		return nil, fmt.Errorf("signature verification failed")
	}

	// Cache the verified index.
	if f.CacheDir != "" {
		f.cacheIndex(url, body)
	}

	return signed.Index, nil
}

// LoadCachedIndex loads a previously cached index if available.
func (f *IndexFetcher) LoadCachedIndex(url string) (*Index, error) {
	if f.CacheDir == "" {
		return nil, fmt.Errorf("no cache directory configured")
	}

	cachePath := f.cachePath(url)
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, err
	}

	var signed SignedIndex
	if err := json.Unmarshal(data, &signed); err != nil {
		return nil, err
	}

	return signed.Index, nil
}

func (f *IndexFetcher) cacheIndex(url string, data []byte) {
	os.MkdirAll(f.CacheDir, 0700)
	os.WriteFile(f.cachePath(url), data, 0600)
}

func (f *IndexFetcher) cachePath(url string) string {
	hash := sha256.Sum256([]byte(url))
	return f.CacheDir + "/" + hex.EncodeToString(hash[:8]) + ".json"
}

// ArtifactVerifier verifies runtime artifact integrity.
type ArtifactVerifier struct {
	// Expected checksums: filename -> hex-encoded SHA-256.
	ExpectedChecksums map[string]string
}

// NewArtifactVerifier creates a verifier with expected checksums.
func NewArtifactVerifier(checksums map[string]string) *ArtifactVerifier {
	return &ArtifactVerifier{ExpectedChecksums: checksums}
}

// VerifyFile checks a file against its expected SHA-256 checksum.
func (v *ArtifactVerifier) VerifyFile(path string) error {
	expected, ok := v.ExpectedChecksums[path]
	if !ok {
		return fmt.Errorf("no expected checksum for %s", path)
	}

	actual, err := FileSHA256(path)
	if err != nil {
		return err
	}

	if actual != expected {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", path, expected, actual)
	}

	return nil
}

// VerifyDir checks all files in a directory against expected checksums.
func (v *ArtifactVerifier) VerifyDir(dir string) error {
	for filename, expected := range v.ExpectedChecksums {
		path := dir + "/" + filename
		actual, err := FileSHA256(path)
		if err != nil {
			return fmt.Errorf("verify %s: %w", filename, err)
		}
		if actual != expected {
			return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", filename, expected, actual)
		}
	}
	return nil
}

// FileSHA256 computes the SHA-256 hex digest of a file.
func FileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// ComputeChecksum computes SHA-256 of arbitrary data.
func ComputeChecksum(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// Signer signs index payloads using Ed25519.
type Signer struct {
	PrivateKey ed25519.PrivateKey
	KeyID      string
}

// SignIndex signs an index and returns a SignedIndex.
func (s *Signer) SignIndex(idx *Index) (*SignedIndex, error) {
	indexJSON, err := json.Marshal(idx)
	if err != nil {
		return nil, fmt.Errorf("marshal index: %w", err)
	}

	ts := time.Now().UTC().Format(time.RFC3339)
	payload := append(indexJSON, []byte(ts)...)

	sig := ed25519.Sign(s.PrivateKey, payload)

	return &SignedIndex{
		Index:     idx,
		Signature: hex.EncodeToString(sig),
		KeyID:     s.KeyID,
		Timestamp: ts,
	}, nil
}

// GenerateKeyPair generates a new Ed25519 key pair for signing.
func GenerateKeyPair() (ed25519.PublicKey, ed25519.PrivateKey) {
	pub, priv, _ := ed25519.GenerateKey(nil) // crypto/rand
	return pub, priv
}

// PublicKeyToBytes serializes a public key for storage.
func PublicKeyToBytes(pub ed25519.PublicKey) []byte {
	return []byte(pub)
}

// PublicKeyFromBytes deserializes a public key.
func PublicKeyFromBytes(b []byte) ed25519.PublicKey {
	return ed25519.PublicKey(b)
}
