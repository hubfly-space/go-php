package runtime

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSignAndVerify(t *testing.T) {
	_, priv := GenerateKeyPair()
	signer := &Signer{PrivateKey: priv, KeyID: "test-key"}

	idx := &Index{
		Runtimes: []Manifest{
			{
				Version:  "8.3.0",
				Platform: "linux",
				Arch:     "amd64",
			},
		},
	}

	signed, err := signer.SignIndex(idx)
	if err != nil {
		t.Fatal(err)
	}

	if signed.KeyID != "test-key" {
		t.Errorf("expected key ID test-key, got %s", signed.KeyID)
	}
	if signed.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}
}

func TestArtifactVerifier_File(t *testing.T) {
	dir := t.TempDir()

	content := []byte("hello world")
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, content, 0644)

	expected := ComputeChecksum(content)

	// Key should match the path passed to VerifyFile.
	verifier := NewArtifactVerifier(map[string]string{
		path: expected,
	})

	if err := verifier.VerifyFile(path); err != nil {
		t.Errorf("verify failed: %v", err)
	}
}

func TestArtifactVerifier_BadChecksum(t *testing.T) {
	dir := t.TempDir()

	content := []byte("hello world")
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, content, 0644)

	verifier := NewArtifactVerifier(map[string]string{
		path: "0000000000000000000000000000000000000000000000000000000000000000",
	})

	if err := verifier.VerifyFile(path); err == nil {
		t.Error("expected checksum mismatch error")
	}
}

func TestArtifactVerifier_Dir(t *testing.T) {
	dir := t.TempDir()

	files := map[string][]byte{
		"a.txt": []byte("file a"),
		"b.txt": []byte("file b"),
	}

	checksums := make(map[string]string)
	for name, content := range files {
		path := filepath.Join(dir, name)
		os.WriteFile(path, content, 0644)
		checksums[name] = ComputeChecksum(content)
	}

	verifier := NewArtifactVerifier(checksums)
	if err := verifier.VerifyDir(dir); err != nil {
		t.Errorf("verify dir failed: %v", err)
	}
}

func TestComputeChecksum(t *testing.T) {
	data := []byte("test data")
	checksum := ComputeChecksum(data)

	if len(checksum) != 64 {
		t.Errorf("expected 64-char hex string, got %d chars", len(checksum))
	}

	checksum2 := ComputeChecksum(data)
	if checksum != checksum2 {
		t.Error("expected deterministic checksums")
	}
}

func TestFileSHA256(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.bin")

	content := []byte{0x00, 0x01, 0x02, 0x03}
	os.WriteFile(path, content, 0644)

	checksum, err := FileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}

	expected := ComputeChecksum(content)
	if checksum != expected {
		t.Errorf("expected %s, got %s", expected, checksum)
	}
}

func TestIndexFetcher_CacheAndLoad(t *testing.T) {
	dir := t.TempDir()
	fetcher := NewIndexFetcher(nil, dir)
	fetcher.Client = nil

	idx := &Index{
		Runtimes: []Manifest{
			{Version: "8.2.0", Platform: "linux", Arch: "amd64"},
		},
	}

	signed := &SignedIndex{
		Index:     idx,
		Signature: "abc123",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	data, _ := json.Marshal(signed)
	url := "https://example.com/index.json"
	cachePath := fetcher.cachePath(url)
	os.MkdirAll(dir, 0700)
	os.WriteFile(cachePath, data, 0600)

	loaded, err := fetcher.LoadCachedIndex(url)
	if err != nil {
		t.Fatal(err)
	}

	if len(loaded.Runtimes) != 1 {
		t.Errorf("expected 1 runtime, got %d", len(loaded.Runtimes))
	}
}

func TestIndexFetcher_FetchHTTP(t *testing.T) {
	pub, priv := GenerateKeyPair()

	idx := &Index{
		Runtimes: []Manifest{
			{Version: "8.3.0", Platform: "linux", Arch: "amd64"},
		},
	}

	signer := &Signer{PrivateKey: priv, KeyID: "test"}
	signed, _ := signer.SignIndex(idx)
	signedJSON, _ := json.Marshal(signed)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(signedJSON)
	}))
	defer server.Close()

	fetcher := NewIndexFetcher(pub, t.TempDir())
	loaded, err := fetcher.FetchAndVerify(server.URL + "/index.json")
	if err != nil {
		t.Fatal(err)
	}

	if len(loaded.Runtimes) != 1 {
		t.Errorf("expected 1 runtime, got %d", len(loaded.Runtimes))
	}
}

func TestIndexFetcher_TamperedSignature(t *testing.T) {
	pub, _ := GenerateKeyPair()

	idx := &Index{
		Runtimes: []Manifest{
			{Version: "8.3.0", Platform: "linux", Arch: "amd64"},
		},
	}

	signed := &SignedIndex{
		Index:     idx,
		Signature: "0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	signedJSON, _ := json.Marshal(signed)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(signedJSON)
	}))
	defer server.Close()

	fetcher := NewIndexFetcher(pub, t.TempDir())
	_, err := fetcher.FetchAndVerify(server.URL + "/index.json")
	if err == nil {
		t.Error("expected signature verification to fail")
	}
}

func TestPublicKeyToAndFromBytes(t *testing.T) {
	pub, _ := GenerateKeyPair()

	b := PublicKeyToBytes(pub)
	got := PublicKeyFromBytes(b)

	if len(got) != len(pub) {
		t.Errorf("key length mismatch: %d != %d", len(got), len(pub))
	}
}
