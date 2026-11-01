package autorun

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestLaunchAgentIsValidXMLAndCarriesTheAutostartFlag(t *testing.T) {
	exe := "/Users/a & b/Applications/CopyNote <test>.app/Contents/MacOS/CopyNote"
	plist := launchAgent("dev.copynote.CopyNote", exe)
	var doc struct {
		Dict struct {
			Strings []string `xml:"string"`
			Array   struct {
				Strings []string `xml:"string"`
			} `xml:"array"`
		} `xml:"dict"`
	}
	if err := xml.Unmarshal(plist, &doc); err != nil {
		t.Fatalf("plist does not parse: %v\n%s", err, plist)
	}
	args := doc.Dict.Array.Strings
	if len(args) != 2 || args[0] != exe || args[1] != AutostartFlag {
		t.Fatalf("ProgramArguments = %q, want [%q %q]", args, exe, AutostartFlag)
	}
	if !strings.Contains(string(plist), "<key>RunAtLoad</key>\n\t<true/>") {
		t.Fatal("the agent must run at sign-in")
	}
}
