package reportfonts

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestStageDOCXFonts(t *testing.T) {
	// Synthetic bytes exercise the package format without distributing a font.
	clear := append([]byte("\x00\x01\x00\x00"), bytes.Repeat([]byte{0xA5}, 64)...)
	obfuscated := bytes.Clone(clear)
	for index := range obfuscated[:32] {
		obfuscated[index] ^= byte(15 - index%16)
	}
	docRels := "<Relationships><Relationship Type=\"" + relationNS + "/fontTable\" Target=\"/parts/fonts.xml\"/></Relationships>"
	font := func(name string) string {
		return "<w:font w:name=\"" + name + "\"><w:embedRegular r:id=\"face\" w:fontKey=\"{00010203-0405-0607-0809-0A0B0C0D0E0F}\"/></w:font>"
	}
	for _, tc := range []struct {
		name, table, target, mode string
		symbol, emoji, invalid    bool
	}{
		{name: "both", table: font(Symbol) + font(Emoji), target: "../data/face.odttf", symbol: true, emoji: true},
		{name: "symbol only", table: font(Symbol), target: "/data/face.odttf", symbol: true},
		{name: "emoji only", table: font(Emoji), target: "/data/face.odttf", emoji: true},
		{name: "unrelated", table: font("Unrelated"), mode: "synthetic-invalid-xml"},
		{name: "declared only", table: "<w:font w:name=\"Segoe UI Symbol\"/>"},
		{name: "external", table: font(Symbol), target: "https://example.com/font.ttf", mode: "External", invalid: true},
		{name: "package escape", table: font(Emoji), target: "../../data/face.odttf", invalid: true},
		{name: "damaged reference", table: font(Symbol), target: "missing.odttf", invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			table := "<w:fonts xmlns:w=\"" + wordNS + "\" xmlns:r=\"" + relationNS + "\">" + tc.table + "</w:fonts>"
			fontRels := "<Relationships><Relationship Id=\"face\" Type=\"" + relationNS +
				"/font\" Target=\"" + tc.target + "\" TargetMode=\"" + tc.mode + "\"/></Relationships>"
			if tc.mode == "synthetic-invalid-xml" {
				fontRels = "<"
			}
			var pkg bytes.Buffer
			zw := zip.NewWriter(&pkg)
			for part, contents := range map[string][]byte{
				"word/_rels/document.xml.rels": []byte(docRels),
				"parts/fonts.xml":              []byte(table),
				"parts/_rels/fonts.xml.rels":   []byte(fontRels),
				"data/face.odttf":              obfuscated,
			} {
				w, err := zw.Create(part)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := w.Write(contents); err != nil {
					t.Fatal(err)
				}
			}
			if err := zw.Close(); err != nil {
				t.Fatal(err)
			}
			zr, err := zip.NewReader(bytes.NewReader(pkg.Bytes()), int64(pkg.Len()))
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			got, err := Stage(t.Context(), zr, dir)
			if tc.invalid {
				if err == nil {
					t.Fatal("invalid reference was admitted")
				}
				return
			}
			if err != nil || got.Symbol != tc.symbol || got.Emoji != tc.emoji || (got.Directory == dir) != (tc.symbol || tc.emoji) {
				t.Fatalf("unexpected selection %+v (%v)", got, err)
			}
			for face, wanted := range map[string]bool{Symbol: tc.symbol, Emoji: tc.emoji} {
				b, err := os.ReadFile(filepath.Join(dir, face+".ttf"))
				if wanted && (err != nil || !bytes.Equal(b, clear)) {
					t.Fatalf("did not restore %s (%v)", face, err)
				}
				if !wanted && !os.IsNotExist(err) {
					t.Fatalf("unexpected staged %s (%v)", face, err)
				}
			}
		})
	}
}
