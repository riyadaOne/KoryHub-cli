package graph

import (
	"slices"
	"testing"
)

func hasLink(r Result, from, to, kind string) bool {
	return slices.Contains(r.Links, Link{From: from, To: to, Kind: kind})
}

func kinds(r Result) map[string]string {
	out := map[string]string{}
	for _, c := range r.Components {
		out[c.ID] = c.Kind
	}

	return out
}

func TestPHPComponentsAndLinks(t *testing.T) {
	r := Analyze(map[string]string{
		"src/PaymentService.php": `<?php
namespace App\Payments;

use App\Payments\Contracts\PaymentGateway;

// PaymentStatus est cité ici dans un commentaire : ignoré
final class PaymentService
{
    use RetriesPayments;

    public function __construct(private PaymentGateway $gateway, private PendingPaymentQueue $queue) {}

    public function pay(): PaymentStatus { return PaymentStatus::Pending; }
}`,
		"src/Contracts/PaymentGateway.php":    "<?php\ninterface PaymentGateway\n{\n}\n",
		"src/Gateways/AirtelMoneyGateway.php": "<?php\nfinal class AirtelMoneyGateway extends BaseGateway implements \\App\\Payments\\Contracts\\PaymentGateway\n{\n}\n",
		"src/Gateways/BaseGateway.php":        "<?php\nabstract class BaseGateway\n{\n}\n",
		"src/PaymentStatus.php":               "<?php\nenum PaymentStatus: string\n{\n    case Pending = 'pending';\n}\n",
		"src/RetriesPayments.php":             "<?php\ntrait RetriesPayments\n{\n}\n",
		"src/PendingPaymentQueue.php":         "<?php\nfinal class PendingPaymentQueue\n{\n}\n",
		"src/helpers.php":                     "<?php\nfunction format_amount(int $xaf): string { return ''; }\n",
		"docs/notes.txt":                      "rien",
	})

	want := map[string]string{
		"PaymentService": "class", "PaymentGateway": "interface", "AirtelMoneyGateway": "class", "BaseGateway": "class",
		"PaymentStatus": "enum", "RetriesPayments": "trait", "PendingPaymentQueue": "class", "format_amount": "function",
	}
	got := kinds(r)
	for id, kind := range want {
		if got[id] != kind {
			t.Errorf("%s : %q ; attendu %q", id, got[id], kind)
		}
	}

	for _, l := range [][3]string{
		{"AirtelMoneyGateway", "BaseGateway", "extends"},
		{"AirtelMoneyGateway", "PaymentGateway", "implements"},
		{"PaymentService", "RetriesPayments", "uses"},
		{"PaymentService", "PaymentGateway", "uses"},
		{"PaymentService", "PendingPaymentQueue", "uses"},
		{"PaymentService", "PaymentStatus", "uses"},
	} {
		if !hasLink(r, l[0], l[1], l[2]) {
			t.Errorf("lien absent %v ; liens %v", l, r.Links)
		}
	}
	if !slices.Equal(r.Unrecognized, []string{"docs/notes.txt"}) {
		t.Errorf("non reconnus : %v", r.Unrecognized)
	}
}

func TestTypeScriptAndModules(t *testing.T) {
	r := Analyze(map[string]string{
		"src/sync.service.ts": `import { OfflineQueue } from './queue';
import { formatXaf } from './money';
export class SyncService implements Syncable {
  constructor(private queue: OfflineQueue) {}
  label() { return formatXaf(100); }
}`,
		"src/syncable.ts": "export interface Syncable {\n  sync(): Promise<void>;\n}\n",
		"src/queue.ts":    "export class OfflineQueue {\n}\n",
		"src/money.ts":    "export const formatXaf = (amount: number) => `${amount} FCFA`;\n",
		"src/config.ts":   "const timeout = 30;\n",
	})

	got := kinds(r)
	if got["SyncService"] != "class" || got["Syncable"] != "interface" || got["formatXaf"] != "function" || got["config"] != "module" {
		t.Errorf("composants : %v", got)
	}
	if !hasLink(r, "SyncService", "Syncable", "implements") || !hasLink(r, "SyncService", "OfflineQueue", "uses") || !hasLink(r, "SyncService", "formatXaf", "uses") {
		t.Errorf("liens : %v", r.Links)
	}
}

func TestDartClassesMixinsAndEnums(t *testing.T) {
	r := Analyze(map[string]string{
		"lib/receipt_printer.dart": "class ReceiptPrinter extends BasePrinter with Retry implements Printer {\n  final ReceiptFormat format = ReceiptFormat.short;\n}\n",
		"lib/base_printer.dart":    "abstract class BasePrinter {\n}\n",
		"lib/printer.dart":         "abstract interface class Printer {\n}\n",
		"lib/retry.dart":           "mixin Retry {\n}\n",
		"lib/receipt_format.dart":  "enum ReceiptFormat { short, long }\n",
	})

	for _, l := range [][3]string{
		{"ReceiptPrinter", "BasePrinter", "extends"},
		{"ReceiptPrinter", "Retry", "uses"},
		{"ReceiptPrinter", "Printer", "implements"},
		{"ReceiptPrinter", "ReceiptFormat", "uses"},
	} {
		if !hasLink(r, l[0], l[1], l[2]) {
			t.Errorf("lien absent %v ; liens %v", l, r.Links)
		}
	}
	if kinds(r)["Retry"] != "trait" {
		t.Errorf("composants : %v", kinds(r))
	}
}

func TestOnlyLinksBetweenComponentsOfTheKoryAreKept(t *testing.T) {
	r := Analyze(map[string]string{
		"src/A.php": "<?php\nuse Illuminate\\Support\\Facades\\Http;\nfinal class A extends Model\n{\n    public function b(): B { return Http::get(''); }\n}\n",
		"src/B.php": "<?php\nfinal class B\n{\n}\n",
	})

	if len(r.Links) != 1 || !hasLink(r, "A", "B", "uses") {
		t.Errorf("liens : %v", r.Links)
	}
}
