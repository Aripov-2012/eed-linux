package cli

import (
	"testing"
)

func TestDataDirPersistentFlagIsInheritedByPlanCommand(t *testing.T) {
	root := NewRootCmd()
	plan, _, err := root.Find([]string{"plan"})
	if err != nil {
		t.Fatal(err)
	}
	if err := root.PersistentFlags().Set("data-dir", "/tmp/eed-components"); err != nil {
		t.Fatal(err)
	}
	value, err := readDataDirFlag(plan)
	if err != nil {
		t.Fatal(err)
	}
	if value != "/tmp/eed-components" {
		t.Fatalf("plan should receive --data-dir from the root command, got %q", value)
	}
}

func TestDataDirPersistentFlagIsInheritedBySubcommands(t *testing.T) {
	for _, cmdName := range []string{"assess", "install", "testdrive"} {
		root := NewRootCmd()
		cmd, _, err := root.Find([]string{cmdName})
		if err != nil {
			t.Fatal(err)
		}
		if err := root.PersistentFlags().Set("data-dir", "/tmp/custom-components"); err != nil {
			t.Fatal(err)
		}
		value, err := readDataDirFlag(cmd)
		if err != nil {
			t.Fatal(err)
		}
		if value != "/tmp/custom-components" {
			t.Fatalf("%s should inherit --data-dir, got %q", cmdName, value)
		}
	}
}
