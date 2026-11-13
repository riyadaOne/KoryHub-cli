package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"koryhub.dev/kory/internal/lock"
)

// installed : projet où @moussa/paiement 1.2.0 est installé dans kory/paiement.
func installed(t *testing.T) (*fakeHub, *session, string) {
	t.Helper()
	hub := newFakeHub(t, map[string]string{
		"README.md":          "# paiement\n",
		"src/Service.php":    "<?php // 1.2.0\n",
		"src/Old.php":        "<?php // ancien\n",
		"src/Customized.php": "<?php // 1.2.0\n",
	})
	s := newSession(t, hub.server.URL)
	root := emptyProject(t)
	if code, out, errOut := s.run("", "pull", "@moussa/paiement", "--yes"); code != ExitOK {
		t.Fatalf("pull : %d, %q, %q", code, out, errOut)
	}

	return hub, s, root
}

func TestUpdateReplacesUntouchedFilesKeepsYoursAndDropsRemovedOnes(t *testing.T) {
	hub, s, root := installed(t)
	os.WriteFile(filepath.Join(root, "kory", "paiement", "src", "Customized.php"), []byte("<?php // à moi\n"), 0o644)

	hub.version = "1.3.0"
	hub.files = map[string]string{
		"README.md":          "# paiement\n",
		"src/Service.php":    "<?php // 1.3.0\n",
		"src/Customized.php": "<?php // 1.3.0\n",
		"src/New.php":        "<?php // nouveau\n",
	}

	code, out, errOut := s.run("", "update", "@moussa/paiement")
	if code != ExitUsage || !strings.Contains(errOut, "--yes") {
		t.Fatalf("sans terminal ni --yes : %d, %q", code, errOut)
	}

	code, out, errOut = s.run("", "update", "@moussa/paiement", "--yes")
	if code != ExitOK {
		t.Fatalf("update : %d, %q, %q", code, out, errOut)
	}
	for _, want := range []string{"@moussa/paiement  1.2.0 → 1.3.0", "Existing files kept: 1", "kory.lock updated"} {
		if !strings.Contains(out, want) {
			t.Errorf("sortie sans %q : %q", want, out)
		}
	}
	if got := readFile(t, root, "kory/paiement/src/Service.php"); got != "<?php // 1.3.0\n" {
		t.Errorf("fichier inchangé non mis à jour : %q", got)
	}
	if got := readFile(t, root, "kory/paiement/src/Customized.php"); got != "<?php // à moi\n" {
		t.Errorf("fichier modifié écrasé : %q", got)
	}
	if readFile(t, root, "kory/paiement/src/New.php") == "" {
		t.Error("nouveau fichier absent")
	}
	if _, err := os.Stat(filepath.Join(root, "kory", "paiement", "src", "Old.php")); err == nil {
		t.Error("fichier retiré par la nouvelle version non supprimé")
	}

	l, _ := lock.Load(root)
	if l.Modules["@moussa/paiement"].Version != "1.3.0" {
		t.Errorf("kory.lock : %+v", l.Modules["@moussa/paiement"])
	}

	code, out, _ = s.run("", "update", "@moussa/paiement", "--yes")
	if code != ExitOK || !strings.Contains(out, "is up to date (1.3.0)") {
		t.Errorf("déjà à jour : %d, %q", code, out)
	}

	journal, _ := os.ReadFile(filepath.Join(root, ".kory", "sync.log"))
	lines := strings.Split(strings.TrimSpace(string(journal)), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], " pull @moussa/paiement 1.2.0 kory/paiement") || !strings.Contains(lines[1], " update @moussa/paiement 1.3.0 kory/paiement") {
		t.Errorf("journal : %q", journal)
	}
}

func TestUpdateOfAKoryThatIsNotInstalled(t *testing.T) {
	hub := newFakeHub(t, koryFiles)
	s := newSession(t, hub.server.URL)
	emptyProject(t)

	code, _, errOut := s.run("", "update", "@moussa/paiement", "--yes")
	if code != ExitUsage || !strings.Contains(errOut, "kory pull @moussa/paiement") {
		t.Errorf("%d, %q", code, errOut)
	}
}

func TestRemoveDeletesTheKoryButKeepsYourModifiedFiles(t *testing.T) {
	_, s, root := installed(t)
	os.WriteFile(filepath.Join(root, "kory", "paiement", "src", "Customized.php"), []byte("<?php // à moi\n"), 0o644)

	code, out, errOut := s.run("", "remove", "@moussa/paiement", "--yes")
	if code != ExitOK {
		t.Fatalf("remove : %d, %q, %q", code, out, errOut)
	}
	for _, want := range []string{"Modified by you since the installation", "src/Customized.php", "Files deleted: 3", "Kept: src/Customized.php", "Removed from kory.lock"} {
		if !strings.Contains(out, want) {
			t.Errorf("sortie sans %q : %q", want, out)
		}
	}
	if readFile(t, root, "kory/paiement/src/Customized.php") != "<?php // à moi\n" {
		t.Error("fichier modifié supprimé")
	}
	if _, err := os.Stat(filepath.Join(root, "kory", "paiement", "README.md")); err == nil {
		t.Error("fichier du kory non supprimé")
	}
	l, _ := lock.Load(root)
	if _, still := l.Modules["@moussa/paiement"]; still {
		t.Error("toujours dans kory.lock")
	}
}

func TestRemoveDeletesEmptyFolders(t *testing.T) {
	_, s, root := installed(t)

	if code, out, errOut := s.run("", "remove", "@moussa/paiement", "--yes"); code != ExitOK {
		t.Fatalf("%d, %q, %q", code, out, errOut)
	}
	if _, err := os.Stat(filepath.Join(root, "kory", "paiement")); err == nil {
		t.Error("dossier vide gardé")
	}
}

func TestAuditShowsTheStateAndFailsOnARemovedVersion(t *testing.T) {
	hub, s, root := installed(t)
	l, _ := lock.Load(root)
	l.Modules["@awa-diop/recu-sms"] = lock.Entry{Version: "0.4.0", Directory: "kory/recu-sms"}
	l.Save(root)

	hub.audit = `{"results":[
		{"name":"@awa-diop/recu-sms","version":"0.4.0","state":"removed","latest_version":null,"message":null},
		{"name":"@moussa/paiement","version":"1.2.0","state":"deprecated","latest_version":"1.3.0","message":"Double débit possible."}]}`

	code, out, errOut := s.run("", "audit")
	if code != ExitError {
		t.Fatalf("code %d, %q, %q", code, out, errOut)
	}
	for _, want := range []string{"@awa-diop/recu-sms", "removed", "kory remove @awa-diop/recu-sms", "deprecated", "kory update @moussa/paiement → 1.3.0", "Double débit possible."} {
		if !strings.Contains(out, want) {
			t.Errorf("sortie sans %q : %q", want, out)
		}
	}
	if !strings.Contains(errOut, "Exit code 1") {
		t.Errorf("erreur : %q", errOut)
	}
	if strings.Join(hub.audited, ",") != "@awa-diop/recu-sms@0.4.0,@moussa/paiement@1.2.0" {
		t.Errorf("envoyé : %v", hub.audited)
	}

	hub.audit = `{"results":[{"name":"@moussa/paiement","version":"1.2.0","state":"update_available","latest_version":"1.3.0","message":null}]}`
	code, out, _ = s.run("", "audit")
	if code != ExitOK || !strings.Contains(out, "update 1.3.0") || !strings.Contains(out, "Kory checked: 1") {
		t.Errorf("mise à jour seule : %d, %q", code, out)
	}
}

func TestAuditWithAnEmptyLock(t *testing.T) {
	hub := newFakeHub(t, koryFiles)
	s := newSession(t, hub.server.URL)
	emptyProject(t)

	code, out, _ := s.run("", "audit")
	if code != ExitOK || !strings.Contains(out, "No kory in kory.lock") || len(hub.audited) != 0 {
		t.Errorf("%d, %q", code, out)
	}
}

func TestPullRemindsOtherUpdatesUnlessTurnedOff(t *testing.T) {
	hub, s, root := installed(t)
	l, _ := lock.Load(root)
	l.Modules["@wanjiru/mpesa-stk"] = lock.Entry{Version: "1.3.2", Directory: "kory/mpesa-stk"}
	delete(l.Modules, "@moussa/paiement")
	l.Save(root)
	os.RemoveAll(filepath.Join(root, "kory", "paiement"))
	hub.audit = `{"results":[{"name":"@wanjiru/mpesa-stk","version":"1.3.2","state":"update_available","latest_version":"1.4.0","message":null}]}`

	code, out, _ := s.run("", "pull", "@moussa/paiement", "--yes")
	if code != ExitOK || !strings.Contains(out, "Other installed kory to look at: 1") {
		t.Fatalf("rappel : %d, %q", code, out)
	}

	s.run("", "config", "set", "update-notify", "deprecations")
	l, _ = lock.Load(root)
	delete(l.Modules, "@moussa/paiement")
	l.Save(root)
	os.RemoveAll(filepath.Join(root, "kory", "paiement"))
	_, out, _ = s.run("", "pull", "@moussa/paiement", "--yes")
	if strings.Contains(out, "Other installed kory") {
		t.Errorf("rappel malgré update-notify deprecations : %q", out)
	}

	code, _, errOut := s.run("", "config", "set", "update-notify", "parfois")
	if code != ExitUsage || !strings.Contains(errOut, "all, deprecations, none") {
		t.Errorf("niveau inconnu : %d, %q", code, errOut)
	}
}
