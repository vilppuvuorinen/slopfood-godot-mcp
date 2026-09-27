package gdcover

import (
	"reflect"
	"testing"
)

func TestKeyEnvSuffixesNamedVars(t *testing.T) {
	env := []string{
		"PATH=/bin",
		"GDCOVER_FILE=/tmp/hits.txt",
		"GDCOVER_VALIDATE_FILE=/tmp/validate.txt",
		"EMPTY=",
		"UNNAMED=1",
	}
	got := KeyEnv(env, "42", "GDCOVER_FILE", "GDCOVER_VALIDATE_FILE")
	want := []string{
		"PATH=/bin",
		"GDCOVER_FILE=/tmp/hits.txt.42",
		"GDCOVER_VALIDATE_FILE=/tmp/validate.txt.42",
		"EMPTY=",
		"UNNAMED=1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("KeyEnv = %v, want %v", got, want)
	}
	if env[1] != "GDCOVER_FILE=/tmp/hits.txt" {
		t.Fatalf("KeyEnv mutated its input: %v", env)
	}
}

func TestKeyEnvLeavesEmptyValueAlone(t *testing.T) {
	env := []string{"GDCOVER_FILE="}
	got := KeyEnv(env, "42", "GDCOVER_FILE")
	if got[0] != "GDCOVER_FILE=" {
		t.Fatalf("KeyEnv = %v, want the empty value untouched", got)
	}
}
