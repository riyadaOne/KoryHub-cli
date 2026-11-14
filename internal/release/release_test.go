package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTargetsCoverThreeSystemsAndTwoArchitectures(t *testing.T) {
	if len(Targets) != 6 {
		t.Fatalf("%d cibles", len(Targets))
	}
	if got := BinaryName(Target{"windows", "arm64"}); got != "kory_windows_arm64.exe" {
		t.Errorf("nom : %q", got)
	}
	if got := BinaryName(Target{"linux", "amd64"}); got != "kory_linux_amd64" {
		t.Errorf("nom : %q", got)
	}
	if !strings.Contains(LDFlags("0.7.0"), "internal/version.Version=0.7.0") {
		t.Errorf("ldflags : %q", LDFlags("0.7.0"))
	}
}

func TestChecksumsUseTheSha256sumFormat(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "kory_linux_amd64"), []byte("b"), 0o644)
	os.WriteFile(filepath.Join(dir, "kory_darwin_arm64"), []byte("a"), 0o644)

	if err := WriteChecksums(dir, []string{"kory_linux_amd64", "kory_darwin_arm64"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "checksums.txt"))
	want := "ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb  kory_darwin_arm64\n" +
		"3e23e8160039594a33894f6564e1b1348bbd7a0088d42c4acb73eeaed59c009d  kory_linux_amd64\n"
	if string(data) != want {
		t.Fatalf("checksums.txt :\n%s", data)
	}
}
