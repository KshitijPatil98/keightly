//go:build !envtest
// +build !envtest

package v1alpha1_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKeightlyConfigCRDSchemaDoesNotContainGlobalCooldown(t *testing.T) {
	crdPath := filepath.Join("..", "..", "..", "config", "crd", "bases", "keightly.io_keightlyconfigs.yaml")
	data, err := os.ReadFile(crdPath)
	if err != nil {
		t.Fatalf("failed reading KeightlyConfig CRD: %v", err)
	}

	if strings.Contains(string(data), "globalCooldown") {
		t.Fatalf("KeightlyConfig CRD schema still contains removed field globalCooldown")
	}
}

func TestKeightlyMonitorCRDSchemaDoesNotContainCooldownOverride(t *testing.T) {
	crdPath := filepath.Join("..", "..", "..", "config", "crd", "bases", "keightly.io_keightlymonitors.yaml")
	data, err := os.ReadFile(crdPath)
	if err != nil {
		t.Fatalf("failed reading KeightlyMonitor CRD: %v", err)
	}

	if strings.Contains(string(data), "cooldownOverride") {
		t.Fatalf("KeightlyMonitor CRD schema still contains removed field cooldownOverride")
	}
}
