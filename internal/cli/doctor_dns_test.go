package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestDoctorChecksDNSAndWebPorts(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := run([]string{"doctor"}, &out, &errOut); err != nil {
		t.Fatalf("doctor: %v (%s)", err, errOut.String())
	}
	text := out.String()
	for _, want := range []string{
		"DNS y puertos web", "lair.test", "127.0.0.1:", "setcap",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("doctor missing %q:\n%s", want, text)
		}
	}
	// Whether the ports are free or not, the advice must be actionable.
	if !strings.Contains(text, "Configuración de Lair") && !strings.Contains(text, "disponible") {
		t.Fatalf("doctor did not report the port status:\n%s", text)
	}
}
