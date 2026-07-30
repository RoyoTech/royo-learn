package cli

import "testing"

func TestCollapseFlag_DefaultsToOn(t *testing.T) {
	t.Setenv("ROYO_LEARN_EXPERIMENTAL_CLI_COLLAPSE", "")
	old := CollapseFlag
	CollapseFlag = "true"
	t.Cleanup(func() { CollapseFlag = old })
	if !ExperimentalCLICollapse() {
		t.Fatal("collapse flag defaults to off, want on")
	}
}

func TestCollapseFlag_EnvOverride(t *testing.T) {
	old := CollapseFlag
	CollapseFlag = "true"
	t.Cleanup(func() { CollapseFlag = old })
	t.Setenv("ROYO_LEARN_EXPERIMENTAL_CLI_COLLAPSE", "false")
	if ExperimentalCLICollapse() {
		t.Fatal("environment override false was ignored")
	}
}

func TestCollapseFlag_InvalidValueFailsSafeOn(t *testing.T) {
	t.Setenv("ROYO_LEARN_EXPERIMENTAL_CLI_COLLAPSE", "not-a-bool")
	if !ExperimentalCLICollapse() {
		t.Fatal("invalid override should fail safe to on")
	}
}
