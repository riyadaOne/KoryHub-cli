# CLAUDE.md — KoryHub · CLI kory

Outil en ligne de commande `kory`, écrit en Go : il publie et intègre des kory depuis le terminal. Spécifications dans `../KoryHub-docs/01-specifications/` (TRD §18, UXUI §7, APPFLOW §6.5) ; règles de l'espace de travail dans `../KoryHub-docs/CLAUDE.md` (elles s'appliquent ici).

## Règles

| Sujet | Règle | Référence |
|---|---|---|
| Module | `koryhub.dev/kory` ; Go fixé dans `go.mod` | TRD §3 |
| Dépendances | Bibliothèque standard d'abord ; toute dépendance est proposée avant d'être ajoutée, et jamais une bibliothèque en C (exécutable unique) | TRD §18 |
| Arborescence | `cmd/kory` (entrée) ; `internal/commands` (une commande par fichier) ; `internal/api`, `config`, `i18n`, `ui`, `semver`, `version` | TRD §4.2 |
| Messages | Aucun texte en dur : `i18n.T("clé")`, clés dans `internal/i18n/messages.go` pour les 4 langues (test qui échoue si une clé manque) ; une ligne de cause, une ligne d'action | UXUI §7 |
| Sortie | `ui.Success`, `ui.Warn`, `ui.Error` ; couleurs seulement dans un terminal et sans `NO_COLOR` ; `ui.Confirm` répond « Non » par défaut et ne pose jamais de question sans terminal (`--yes`) | UXUI §7 |
| Codes de sortie | 0 succès, 1 erreur, 2 mauvaise utilisation | UXUI §7 |
| API | `internal/api` ; en-têtes `User-Agent: kory/x.y.z (os; arch)` et `Accept-Language` ; erreurs `{message, code}` déjà traduites par le serveur ; 426 = mettre à jour kory | TRD §17 |
| Contrat | `api/openapi-v1.json` est copié depuis `KoryHub-web/app/resources/openapi/v1.json` après chaque changement de l'API ; `internal/api/contract_test.go` vérifie que les types Go ont exactement les champs du contrat | T-07 |
| Version | Format `x.y.z`, injectée à la construction (`-ldflags "-X koryhub.dev/kory/internal/version.Version=1.2.3"`) | T-11 |
| Vie privée | Le CLI n'envoie aucune information sur le dossier, Git ou la machine | T-44 |
| Qualité | `gofmt -l .` vide, `go vet ./...`, tests ciblés (`go test ./internal/<paquet> -run TestNom`) | TRD §24 |
| Essai local | `KORY_API_URL=http://api.koryhub.test:8000` ; `KORY_CONFIG_DIR` pour isoler les réglages | TRD §18 |
