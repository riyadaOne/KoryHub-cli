package credentials

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCredentialsAreSavedReadableByTheUserOnlyAndReloaded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kory", "credentials")

	if err := Save(path, Credentials{Token: "kory_abc", APIURL: "https://api.koryhub.test/"}); err != nil {
		t.Fatal(err)
	}

	creds, err := Load(path, "https://api.koryhub.test")
	if err != nil || creds.Token != "kory_abc" {
		t.Fatalf("relu : %+v, %v", creds, err)
	}

	if runtime.GOOS != "windows" {
		info, _ := os.Stat(path)
		if info.Mode().Perm() != 0o600 {
			t.Errorf("droits %v", info.Mode().Perm())
		}
	}
}

func TestMissingFileOrAnotherServerMeansNotLoggedIn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials")

	if _, err := Load(path, "https://api.koryhub.dev"); !errors.Is(err, ErrNotLoggedIn) {
		t.Errorf("fichier absent : %v", err)
	}

	Save(path, Credentials{Token: "kory_abc", APIURL: "http://api.koryhub.test:8000"})
	if _, err := Load(path, "https://api.koryhub.dev"); !errors.Is(err, ErrNotLoggedIn) {
		t.Errorf("autre serveur : %v", err)
	}
}

func TestUnreadableFileIsAnErrorAndDeleteIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials")
	os.WriteFile(path, []byte("pas du json"), 0o600)

	if _, err := Load(path, "https://api.koryhub.dev"); err == nil || errors.Is(err, ErrNotLoggedIn) {
		t.Errorf("fichier illisible : %v", err)
	}

	if err := Delete(path); err != nil {
		t.Fatal(err)
	}
	if err := Delete(path); err != nil {
		t.Errorf("deuxième effacement : %v", err)
	}
}
