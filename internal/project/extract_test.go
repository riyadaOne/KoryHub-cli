package project

import (
	"os"
	"slices"
	"strings"
	"testing"
)

func TestExtractFollowsLinksOutsideThePathUpToThreeLevels(t *testing.T) {
	root := t.TempDir()
	write(t, root, "app/Services/Payment/PaymentService.php", "<?php\nfinal class PaymentService\n{\n    public function __construct(private PendingPaymentQueue $queue) {}\n}\n")
	write(t, root, "app/Queues/PendingPaymentQueue.php", "<?php\nfinal class PendingPaymentQueue\n{\n    public function store(PendingPayment $p) {}\n}\n")
	write(t, root, "app/Models/PendingPayment.php", "<?php\nfinal class PendingPayment\n{\n    public Customer $customer;\n}\n")
	write(t, root, "app/Models/Customer.php", "<?php\nfinal class Customer\n{\n    public Address $address;\n}\n")
	write(t, root, "app/Models/Address.php", "<?php\nfinal class Address\n{\n}\n")
	write(t, root, "vendor/laravel/Model.php", "<?php\nclass Model\n{\n}\n")

	ignore, _ := LoadIgnore(root)
	start, related, err := Extract(root, "app/Services/Payment", ignore)
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(start, []string{"app/Services/Payment/PaymentService.php"}) {
		t.Errorf("départ : %v", start)
	}

	paths := map[string]Related{}
	for _, r := range related {
		paths[r.Path] = r
	}
	if r := paths["app/Queues/PendingPaymentQueue.php"]; r.Depth != 1 || r.Name != "PendingPaymentQueue" || r.From != "app/Services/Payment/PaymentService.php" {
		t.Errorf("niveau 1 : %+v", r)
	}
	if paths["app/Models/PendingPayment.php"].Depth != 2 || paths["app/Models/Customer.php"].Depth != 3 {
		t.Errorf("niveaux : %+v", related)
	}
	if _, ok := paths["app/Models/Address.php"]; ok {
		t.Error("au-delà de 3 niveaux : ne doit pas être proposé")
	}
}

func TestRequirementsComeFromTheProjectManifests(t *testing.T) {
	root := t.TempDir()
	write(t, root, "composer.json", `{"require":{"php":">=8.3","laravel/framework":"^13.0","guzzlehttp/guzzle":"^7.9"}}`)
	write(t, root, "vendor/composer/installed.json", `{"packages":[{"name":"laravel/framework","autoload":{"psr-4":{"Illuminate\\":"src/Illuminate/"}}},{"name":"guzzlehttp/guzzle","autoload":{"psr-4":{"GuzzleHttp\\":"src/"}}}]}`)
	write(t, root, "package.json", `{"dependencies":{"axios":"^1.7.0","@capacitor/core":"^7.0.0"}}`)
	write(t, root, "pubspec.yaml", "name: caisse\ndependencies:\n  flutter:\n    sdk: flutter\n  http: ^1.2.0\n")
	write(t, root, "src/Gateway.php", "<?php\nuse Illuminate\\Support\\Facades\\Http;\nfinal class Gateway {}\n")
	write(t, root, "web/sync.ts", "import axios from 'axios';\nimport { Network } from '@capacitor/core/network';\nimport { x } from './local';\n")
	write(t, root, "lib/api.dart", "import 'package:http/http.dart' as http;\nimport 'package:caisse/models.dart';\n")

	got := DetectRequirements(root, []string{"src/Gateway.php", "web/sync.ts", "lib/api.dart"})

	want := []Requirement{
		{Ecosystem: "composer", Name: "laravel/framework", Version: "^13.0"},
		{Ecosystem: "composer", Name: "php", Version: ">=8.3"},
		{Ecosystem: "npm", Name: "@capacitor/core", Version: "^7.0.0"},
		{Ecosystem: "npm", Name: "axios", Version: "^1.7.0"},
		{Ecosystem: "pub", Name: "http", Version: "^1.2.0"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("prérequis :\n%v\nattendu :\n%v", got, want)
	}
}

func TestManifestKeepsVersionConstraintsReadable(t *testing.T) {
	k := &Kory{ManifestPath: t.TempDir() + "/kory.json", Manifest: Manifest{Name: "a", Requires: []Requirement{{Ecosystem: "composer", Name: "php", Version: ">=8.3"}}}}
	if err := k.Save(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(k.ManifestPath)
	if !strings.Contains(string(data), `"version": ">=8.3"`) {
		t.Errorf("kory.json : %s", data)
	}
}

func TestSharedFilesNameTheKoryThatAlreadyHasThem(t *testing.T) {
	root := t.TempDir()
	write(t, root, "src/Money.php", "<?php\nfinal class Money {}\n")
	write(t, root, ".kory/caisse/kory.json", `{"name":"caisse","version":"1.0.0","files":["src/Money.php"]}`)

	write(t, root, ".kory/aa-nouveau/kory.json", `{"name":"aa-nouveau","version":"1.0.0","files":["src/Money.php"]}`)

	got := SharedFiles(root, []string{"src/Money.php", "src/Other.php"}, "aa-nouveau")
	if !slices.Equal(got, []Shared{{Path: "src/Money.php", Kory: "caisse"}}) {
		t.Errorf("partagés : %v", got)
	}
}
