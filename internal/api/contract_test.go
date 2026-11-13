package api

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// Le contrat OpenAPI est exporté par le serveur (php artisan scramble:export) et copié dans
// api/openapi-v1.json. Ce test vérifie que chaque type Go a exactement les champs du contrat :
// un champ ajouté ou retiré côté serveur fait échouer le test (T-07).

type schema struct {
	Properties map[string]json.RawMessage `json:"properties"`
}

type contract struct {
	Paths map[string]map[string]struct {
		Responses map[string]struct {
			Content map[string]struct {
				Schema schema `json:"schema"`
			} `json:"content"`
		} `json:"responses"`
	} `json:"paths"`
}

func loadContract(t *testing.T) contract {
	t.Helper()

	data, err := os.ReadFile("../../api/openapi-v1.json")
	if err != nil {
		t.Fatalf("contrat introuvable : %v", err)
	}

	var c contract
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}

	return c
}

func responseFields(t *testing.T, c contract, path, method, status string) []string {
	t.Helper()

	operation, ok := c.Paths[path][method]
	if !ok {
		t.Fatalf("%s %s absent du contrat", method, path)
	}

	var fields []string
	for name := range operation.Responses[status].Content["application/json"].Schema.Properties {
		fields = append(fields, name)
	}
	slices.Sort(fields)

	return fields
}

func jsonFields(value any) []string {
	var fields []string
	kind := reflect.TypeOf(value)

	for i := range kind.NumField() {
		name := strings.Split(kind.Field(i).Tag.Get("json"), ",")[0]
		if name != "" && name != "-" {
			fields = append(fields, name)
		}
	}
	slices.Sort(fields)

	return fields
}

func TestMetaMatchesTheContract(t *testing.T) {
	want := responseFields(t, loadContract(t), "/meta", "get", "200")

	if got := jsonFields(Meta{}); !slices.Equal(got, want) {
		t.Errorf("Meta : champs %v ; contrat %v", got, want)
	}
}

func TestAuthTypesMatchTheContract(t *testing.T) {
	c := loadContract(t)

	cases := []struct {
		name, path, method string
		value              any
	}{
		{"DeviceCode", "/auth/device", "post", DeviceCode{}},
		{"IssuedToken", "/auth/device/token", "post", IssuedToken{}},
		{"User", "/user", "get", User{}},
	}

	for _, tc := range cases {
		want := responseFields(t, c, tc.path, tc.method, "200")
		if got := jsonFields(tc.value); !slices.Equal(got, want) {
			t.Errorf("%s : champs %v ; contrat %v", tc.name, got, want)
		}
	}
}
