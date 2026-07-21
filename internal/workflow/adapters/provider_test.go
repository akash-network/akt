package adapters

import (
	"context"
	"os"
	"testing"
)

func TestReadSDLFromPath(t *testing.T) {
	obj, err := readSDL([]byte(testSDLPath))
	if err != nil {
		t.Fatalf("readSDL(path): %v", err)
	}

	mani, err := obj.Manifest()
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	if len(mani) != 1 {
		t.Fatalf("manifest groups = %d, want 1", len(mani))
	}
	if len(mani[0].Services) != 1 || mani[0].Services[0].Name != "web" {
		t.Errorf("manifest services = %+v, want single web service", mani[0].Services)
	}
}

func TestReadSDLFromContent(t *testing.T) {
	content, err := os.ReadFile(testSDLPath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	obj, err := readSDL(content)
	if err != nil {
		t.Fatalf("readSDL(content): %v", err)
	}

	if _, err := obj.Manifest(); err != nil {
		t.Fatalf("Manifest: %v", err)
	}
}

func TestReadSDLErrors(t *testing.T) {
	if _, err := readSDL(nil); err == nil {
		t.Error("expected error for empty SDL")
	}
	if _, err := readSDL([]byte("   \n\t")); err == nil {
		t.Error("expected error for blank SDL")
	}
}

func TestSendManifestRequiresDSeq(t *testing.T) {
	p := NewProviderClient(testClientContext(), "")

	if err := p.SendManifest(context.Background(), testProviderAddr().String(), 0, []byte(testSDLPath)); err == nil {
		t.Error("expected error for zero dseq")
	}
}

func TestLeaseStatusRequiresDSeq(t *testing.T) {
	p := NewProviderClient(testClientContext(), "")

	if _, err := p.LeaseStatus(context.Background(), testProviderAddr().String(), 0); err == nil {
		t.Error("expected error for zero dseq")
	}
}
