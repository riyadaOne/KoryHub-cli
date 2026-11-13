package secrets

import (
	"strings"
	"testing"
)

func TestRulesLoadAndCompileWithTheStandardLibrary(t *testing.T) {
	rs, err := rules()
	if err != nil {
		t.Fatal(err)
	}
	if len(rs.Rules) < 200 {
		t.Errorf("seulement %d règles utilisables", len(rs.Rules))
	}
}

func TestKnownSecretsAreFoundWithoutTheirValue(t *testing.T) {
	cases := map[string]string{
		"koryhub-cli-token":   "$t = 'kory_" + strings.Repeat("A", 40) + "0123abcd';",
		"github-pat":          "token = \"ghp_" + "x8Kq2LmN4pR7sT1vW3yZ5bD6fH9jK0cE2gA4\"",
		"stripe-access-token": "STRIPE = 'sk_live_" + "51HxQ2bKz8mN4pR7sT1vW3yZ'",
	}

	for rule, line := range cases {
		findings, err := Scan("src/config.php", []byte("<?php\n\n"+line+"\n"))
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, f := range findings {
			if f.Rule == rule && f.Line == 3 && f.Path == "src/config.php" {
				found = true
			}
		}
		if !found {
			t.Errorf("%s non trouvé : %+v", rule, findings)
		}
	}
}

func TestOrdinaryCodeIsNotASecret(t *testing.T) {
	content := []byte(`<?php

final class PaymentService
{
    public function pay(string $phone, int $amountXaf, string $orderId): PaymentStatus
    {
        $key = 'order-'.$orderId;
        $apiKey = config('services.airtel.key');
        $token = env('AIRTEL_TOKEN', 'your-token-here');

        return $this->gateway->charge($phone, $amountXaf, $key)->status();
    }
}
`)

	findings, err := Scan("src/PaymentService.php", content)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Errorf("aucun secret attendu : %+v", findings)
	}
}

func TestEntropyMatchesShannon(t *testing.T) {
	if got := entropy("aaaa"); got != 0 {
		t.Errorf("entropie de aaaa : %v", got)
	}
	if got := entropy("ab"); got != 1 {
		t.Errorf("entropie de ab : %v", got)
	}
}
