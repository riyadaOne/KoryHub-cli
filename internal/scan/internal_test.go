package scan

import (
	"testing"
)

func kinds(findings []Finding) map[string]Kind {
	out := map[string]Kind{}
	for _, f := range findings {
		out[f.Value] = f.Kind
	}

	return out
}

func TestInternalInformationIsFound(t *testing.T) {
	content := []byte(`<?php
$callback = 'https://api-prod.boutique-client.td/airtel/callback';
$db = 'mysql://caisse:motdepasse@10.4.2.17:3306/caisse_prod';
$support = 'support@boutique-client.td';
$host = "erp.boutique-client.td";
$server = '41.202.207.5';
DB_DATABASE=caisse_prod
`)

	got := kinds(Lines("src/Gateway.php", content))

	want := map[string]Kind{
		"https://api-prod.boutique-client.td/airtel/callback":  KindURL,
		"mysql://caisse:motdepasse@10.4.2.17:3306/caisse_prod": KindConnection,
		"support@boutique-client.td":                           KindEmail,
		"erp.boutique-client.td":                               KindDomain,
		"41.202.207.5":                                         KindIP,
		"caisse_prod":                                          KindDatabase,
	}
	for value, kind := range want {
		if got[value] != kind {
			t.Errorf("%q : %q ; attendu %q (trouvés %v)", value, got[value], kind, got)
		}
	}
}

func TestExampleValuesAreIgnored(t *testing.T) {
	content := []byte(`<?php
// ex. https://example.com/webhooks/airtel
$url = 'http://localhost:8000/api';
$mail = 'moussa@example.org';
$local = '127.0.0.1';
$doc = '192.0.2.10';
$test = "api.koryhub.test";
require 'PaymentService.php';
DB_DATABASE=your_database
$this->http->post('/payments');
`)

	if got := Lines("src/Gateway.php", content); len(got) != 0 {
		t.Errorf("rien ne devait être signalé : %+v", got)
	}
}

func TestLineNumbersAreReported(t *testing.T) {
	got := Lines("a.php", []byte("<?php\n\n$u = 'https://erp.client.td';\n"))
	if len(got) != 1 || got[0].Line != 3 || got[0].Path != "a.php" {
		t.Errorf("trouvés : %+v", got)
	}
}
