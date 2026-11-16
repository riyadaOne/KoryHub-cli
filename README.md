# kory

**L'outil en ligne de commande de [KoryHub](https://koryhub.site)** : publier et installer des **kory**, des briques de code réutilisables partagées entre développeurs africains.

Écrit en Go, sans aucune dépendance externe. Disponible pour Windows, macOS et Linux, sur processeurs amd64 et arm64. Messages en français, anglais, portugais et arabe.

---

## Installer

**Windows** (PowerShell) :

```
irm https://koryhub.site/install.ps1 | iex
```

**macOS et Linux** :

```
curl -fsSL https://koryhub.site/install.sh | sh
```

Le script télécharge l'exécutable, **vérifie son empreinte SHA-256** (rien n'est installé si elle diffère), l'installe sans droit d'administrateur, puis règle l'adresse de l'API du site qui l'a servi. Ouvrez ensuite un nouveau terminal :

```
kory version
```

## Démarrer

**Installer le kory d'un autre développeur :**

```
kory search mobile money
kory info @moussa/mobile-money-resilient
kory pull @moussa/mobile-money-resilient
```

**Publier le vôtre :**

```
kory login
kory init paiement-orange-money --from app/Services/Payment
kory graph paiement-orange-money
kory push paiement-orange-money
```

`kory init` repère les fichiers liés en dehors du dossier et les prérequis (`composer.json`, `package.json`, `pubspec.yaml`), et prépare un README à compléter : « Problème résolu » et « Pièges évités ».

## Commandes

| Commande | Rôle |
|---|---|
| `kory init <nom>` | Préparer un kory dans ce projet (`--from <dossier>`, `--add <chemin>`) |
| `kory list` | Lister les kory de ce projet |
| `kory graph <nom>` | Détecter l'architecture d'un kory et l'écrire dans `kory.json` |
| `kory push <nom>` | Publier un kory (`--dry-run` pour vérifier sans envoyer) |
| `kory search <mots>` | Chercher des kory |
| `kory info <@auteur/kory>` | Voir un kory avant de l'installer |
| `kory pull <@auteur/kory>` | Installer un kory dans le projet |
| `kory update <@auteur/kory>` | Mettre à jour un kory installé, après confirmation |
| `kory remove <@auteur/kory>` | Retirer un kory du projet |
| `kory audit` | Contrôler les versions de `kory.lock` (code 1 si une version est retirée ou à risque) |
| `kory login` · `logout` · `whoami` | Relier ce terminal à son compte, le déconnecter, voir le compte relié |
| `kory config get\|set <clé>` | Lire ou changer un réglage |
| `kory version` · `help` | Version, aide |

`kory help` affiche l'aide dans la langue du terminal.

## Sécurité

- **Secrets bloqués avant l'envoi** : `kory push` cherche les clés et mots de passe (règles de gitleaks) et refuse la publication s'il en trouve. Le serveur refait la recherche avec gitleaks.
- **Empreintes vérifiées** : chaque archive est contrôlée en SHA-256 au téléchargement ; une archive incomplète n'est jamais écrite.
- **Coupures réseau** : en cas de coupure pendant un téléchargement, `kory` réessaie avant d'abandonner, sans rien écrire.
- **Vos fichiers d'abord** : quand un fichier existe déjà, `kory pull` demande s'il faut le garder, le remplacer ou afficher la différence.
- **Connexion sans mot de passe** : `kory login` passe par un code à autoriser sur le site ; le jeton est révocable depuis le site ou par `kory logout`.
- **Exclusions** : `.koryignore` (même principe que `.gitignore`) ; `.env*`, clés, `.git`, `node_modules` et `vendor` ne sont jamais envoyés.

## Fichiers du projet

| Fichier | Rôle |
|---|---|
| `.kory/<nom>/kory.json` | manifeste d'un kory : nom, version, licence, fichiers, prérequis, architecture |
| `.kory/<nom>/README.md` | README publié avec le kory |
| `kory.lock` | kory installés, versions et empreintes (à versionner) |
| `.koryignore` | fichiers jamais envoyés |

## Réglages

| Clé | Valeurs |
|---|---|
| `lang` | `fr`, `en`, `ar`, `pt` |
| `api-url` | adresse de l'API (réglée par le script d'installation) |
| `credential-file` | fichier où le jeton est rangé |
| `update-notify` | `all`, `deprecations`, `none` : rappels de mise à jour après `kory pull` |

Variables d'environnement : `KORY_API_URL` (prioritaire sur le réglage), `KORY_CONFIG_DIR`, `KORY_UPDATE_CHECK=off` (pas d'avis de nouvelle version), `NO_COLOR`.

## Construire depuis les sources

Go 1.27 :

```
go build -o kory ./cmd/kory
go test ./...
```

Exécutables de publication pour les 6 cibles, avec `checksums.txt` :

```
go run ./scripts/release 0.7.0
```

## Équipe

**Équipe KoryHub, Tchad** — RIYADA ONE TECHNOLOGIES SARL, N'Djamena. Projet présenté à la Coupe d'Afrique des Développeurs (CADEV 2026). Site et API : [riyadaOne/KoryHub-web](https://github.com/riyadaOne/KoryHub-web).

## Licence

© 2026 RIYADA ONE TECHNOLOGIES SARL. Tous droits réservés.

Les règles de détection des secrets (`internal/secrets/rules.json`) viennent de [gitleaks](https://github.com/gitleaks/gitleaks) 8.30.1, sous licence MIT (voir `internal/secrets/NOTICE`).
