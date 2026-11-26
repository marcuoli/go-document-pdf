package report

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"math"
	"strings"
	"testing"
)

func TestCenteredCoverLogoFitsAndPreservesPrimaryMark(t *testing.T) {
	primary := coverPNG(t, 12, 3)
	for _, test := range []struct {
		name                   string
		pixels                 image.Point
		maxWidth               float64
		maxHeight              float64
		wantPosition, wantSize string
	}{
		{"wide", image.Pt(120, 60), 30, 30, "left:90.000000mm;top:67.200000mm", "width:30.000000mm;height:15.000000mm"},
		{"tall", image.Pt(60, 120), 30, 30, "left:97.500000mm;top:67.200000mm", "width:15.000000mm;height:30.000000mm"},
		{"height bound", image.Pt(120, 60), 50, 10, "left:95.000000mm;top:67.200000mm", "width:20.000000mm;height:10.000000mm"},
	} {
		t.Run(test.name, func(t *testing.T) {
			options := Cover{LogoPNG: primary, LogoAlt: "Primary"}
			plain, err := coverHTML("Cover", options)
			if err != nil {
				t.Fatal(err)
			}
			options.CenteredLogo = &CoverLogo{
				PNG: coverPNG(t, test.pixels.X, test.pixels.Y), Alt: "A & B",
				TopMM: 67.2, MaxWidthMM: test.maxWidth, MaxHeightMM: test.maxHeight,
			}
			branded, err := coverHTML("Cover", options)
			if err != nil {
				t.Fatal(err)
			}
			html := string(branded)
			start := strings.Index(html, "<div class=\"cover-centered-logo\"")
			if start < 0 {
				t.Fatal("centered logo is missing")
			}
			end := start + strings.Index(html[start:], "</div>") + len("</div>")
			tag := html[start:end]
			if !strings.Contains(tag, test.wantPosition) || !strings.Contains(tag, test.wantSize) || !strings.Contains(tag, "alt=\"A &amp; B\"") {
				t.Fatalf("wrong placement or alt text in %s", tag)
			}
			if withoutLogo := html[:start] + html[end:]; withoutLogo != string(plain) {
				t.Fatal("adding the centered logo also changed other cover markup")
			}
		})
	}
}

func TestCenteredCoverLogoKeepsOriginalAspectAfterDownsampling(t *testing.T) {
	logo := &CoverLogo{
		PNG: coverPNG(t, 640, 2), TopMM: 67.2, MaxWidthMM: 30, MaxHeightMM: 30,
	}
	for _, test := range []struct {
		name          string
		width, height int
		want          string
	}{
		{"PNG default", 0, 0, "width:30.000000mm;height:0.093750mm"},
		{"original", 4096, 19, "width:30.000000mm;height:0.139160mm"},
	} {
		t.Run(test.name, func(t *testing.T) {
			logo.AspectWidth, logo.AspectHeight = test.width, test.height
			got, err := centeredCoverLogoHTML(logo)
			if err != nil || !strings.Contains(got, test.want) {
				t.Fatalf("want fitted box %q, got %q with %v", test.want, got, err)
			}
		})
	}
}

func TestCenteredCoverLogoInvalidInputFailsWrite(t *testing.T) {
	valid := coverPNG(t, 10, 10)
	for _, test := range []struct {
		name string
		logo CoverLogo
	}{
		{"not PNG", CoverLogo{PNG: []byte("invalid"), TopMM: 67.2, MaxWidthMM: 30, MaxHeightMM: 30}},
		{"partial aspect", CoverLogo{PNG: valid, MaxWidthMM: 30, MaxHeightMM: 30, AspectWidth: 30}},
		{"negative aspect", CoverLogo{PNG: valid, MaxWidthMM: 30, MaxHeightMM: 30, AspectWidth: -2, AspectHeight: 1}},
		{"zero bound", CoverLogo{PNG: valid, MaxHeightMM: 30}},
		{"infinite bound", CoverLogo{PNG: valid, MaxWidthMM: math.Inf(1), MaxHeightMM: 30}},
		{"NaN top", CoverLogo{PNG: valid, TopMM: math.NaN(), MaxWidthMM: 30, MaxHeightMM: 30}},
		{"past page bottom", CoverLogo{PNG: valid, TopMM: 280, MaxWidthMM: 30, MaxHeightMM: 30}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var d Document
			var output bytes.Buffer
			err := d.Source(Options{Cover: &Cover{CenteredLogo: &test.logo}}).Write(t.Context(), &output)
			if err == nil || output.Len() != 0 {
				t.Fatalf("invalid cover logo emitted %d bytes with %v", output.Len(), err)
			}
		})
	}
}

func TestCenteredCoverLogoRejectsOverBudgetPNGHeader(t *testing.T) {
	data := coverPNG(t, 1, 1)
	// Keep the PNG signature and IHDR valid while advertising an oversized bitmap.
	binary.BigEndian.PutUint32(data[16:20], 5000)
	binary.BigEndian.PutUint32(data[20:24], 5000)
	binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))
	logo := &CoverLogo{PNG: data, MaxWidthMM: 20, MaxHeightMM: 20}
	_, err := coverHTML("Cover", Cover{CenteredLogo: logo})
	if err == nil || !strings.Contains(err.Error(), "PNG pixel limit") {
		t.Fatalf("oversized PNG did not fail the pixel limit: %v", err)
	}
}

func coverPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
