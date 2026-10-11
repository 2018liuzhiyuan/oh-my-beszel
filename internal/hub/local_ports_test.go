package hub

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseLocalPortAssignment(t *testing.T) {
	valid, err := parseLocalPortAssignment("h101.json", []byte(`{"name":"h101","port":47100,"active":true}`))
	require.NoError(t, err)
	require.Equal(t, localPortAssignment{Name: "h101", Port: 47100, Active: true}, valid)

	for _, test := range []struct {
		name     string
		filename string
		data     string
	}{
		{"mismatched filename", "h103.json", `{"name":"h101","port":47100,"active":true}`},
		{"invalid port", "h101.json", `{"name":"h101","port":70000,"active":true}`},
		{"zero port", "h101.json", `{"name":"h101","port":0}`},
		{"negative port", "h101.json", `{"name":"h101","port":-1}`},
		{"port just above upper bound", "h101.json", `{"name":"h101","port":65536}`},
		{"fractional port", "h101.json", `{"name":"h101","port":47100.5}`},
		{"string port", "h101.json", `{"name":"h101","port":"47100"}`},
		{"missing port", "h101.json", `{"name":"h101","active":true}`},
		{"empty name", ".json", `{"name":"","port":47100}`},
		{"missing name", "h101.json", `{"port":47100}`},
		{"wrong active type", "h101.json", `{"name":"h101","port":47100,"active":"true"}`},
		{"null record", "h101.json", `null`},
		{"array record", "h101.json", `[]`},
		{"multiple JSON records", "h101.json", `{"name":"h101","port":47100} {}`},
		{"wrong filename extension", "h101.tmp", `{"name":"h101","port":47100}`},
		{"case mismatched name", "H101.json", `{"name":"h101","port":47100}`},
		{"malformed JSON", "h101.json", `{"name":`},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseLocalPortAssignment(test.filename, []byte(test.data))
			require.Error(t, err)
		})
	}
}

func TestParseLocalPortAssignmentBoundaries(t *testing.T) {
	for _, port := range []int{1, 65535} {
		want := localPortAssignment{Name: "h101", Port: port, Active: true}
		data, err := json.Marshal(want)
		require.NoError(t, err)
		got, err := parseLocalPortAssignment("h101.json", data)
		require.NoError(t, err)
		require.Equal(t, want, got)
	}

	data := `{"name":"h101","port":47100}`
	padded := data + strings.Repeat(" ", 1024-len(data))
	got, err := parseLocalPortAssignment("h101.json", []byte(padded))
	require.NoError(t, err)
	require.Equal(t, localPortAssignment{Name: "h101", Port: 47100}, got)
	_, err = parseLocalPortAssignment("h101.json", []byte(padded+" "))
	require.ErrorContains(t, err, "too large")
}

func FuzzParseLocalPortAssignment(f *testing.F) {
	for _, seed := range []struct {
		filename string
		data     string
	}{
		{"h101.json", `{"name":"h101","port":1,"active":true}`},
		{"h101.json", `{"name":"h101","port":65535}`},
		{"h101.json", `{"name":"h101","port":0}`},
		{"h103.json", `{"name":"h101","port":47100}`},
		{"h101.json", `{"name":`},
		{"h101.json", "null"},
		{"h101.json", strings.Repeat(" ", 1025)},
	} {
		f.Add(seed.filename, []byte(seed.data))
	}
	f.Fuzz(func(t *testing.T, filename string, data []byte) {
		assignment, err := parseLocalPortAssignment(filename, data)
		if err != nil {
			return
		}
		if len(data) > 1024 || assignment.Name == "" || filename != assignment.Name+".json" || assignment.Port < 1 || assignment.Port > 65535 {
			t.Fatalf("invalid assignment accepted: %+v", assignment)
		}
		encoded, err := json.Marshal(assignment)
		require.NoError(t, err)
		if len(encoded) > 1024 {
			return
		}
		roundTrip, err := parseLocalPortAssignment(filename, encoded)
		require.NoError(t, err)
		require.Equal(t, assignment, roundTrip)
	})
}
