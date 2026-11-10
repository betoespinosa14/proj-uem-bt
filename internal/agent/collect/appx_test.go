package collect

import (
	"reflect"
	"testing"
)

func TestParseAppxManifest(t *testing.T) {
	teams := `<?xml version="1.0" encoding="utf-8"?>
<Package xmlns="http://schemas.microsoft.com/appx/manifest/foundation/windows10" xmlns:uap="http://schemas.microsoft.com/appx/manifest/uap/windows10">
  <Identity Name="MSTeams" Publisher="CN=Microsoft Corporation" Version="25153.1010.3727.5483" />
  <Properties><DisplayName>Microsoft Teams</DisplayName><PublisherDisplayName>Microsoft Corporation</PublisherDisplayName></Properties>
  <Applications><Application Id="MSTeams" Executable="ms-teams.exe"><uap:VisualElements DisplayName="Microsoft Teams" /></Application></Applications>
</Package>`
	calc := `<Package xmlns="http://schemas.microsoft.com/appx/manifest/foundation/windows10" xmlns:uap="http://schemas.microsoft.com/appx/manifest/uap/windows10">
  <Identity Name="Microsoft.WindowsCalculator" />
  <Properties><DisplayName>ms-resource:AppStoreName</DisplayName><PublisherDisplayName>Microsoft Corporation</PublisherDisplayName></Properties>
  <Applications><Application Executable="CalculatorApp.exe"><uap:VisualElements DisplayName="ms-resource:AppName" /></Application></Applications>
</Package>`
	framework := `<Package><Identity Name="Microsoft.VCLibs.140.00" /><Properties><DisplayName>VCLibs</DisplayName><Framework>true</Framework></Properties></Package>`
	hidden := `<Package xmlns:uap="x"><Identity Name="Contoso.Helper" /><Properties><DisplayName>Helper</DisplayName></Properties>
  <Applications><Application Executable="h.exe"><uap:VisualElements DisplayName="Helper" AppListEntry="none" /></Application></Applications></Package>`

	cases := []struct {
		xml  string
		want appxApp
		ok   bool
	}{
		{teams, appxApp{Name: "Microsoft Teams", Fallback: "Microsoft Teams", Publisher: "Microsoft Corporation"}, true},
		{calc, appxApp{Name: "ms-resource:AppStoreName", Fallback: "WindowsCalculator", Publisher: "Microsoft Corporation"}, true},
		{framework, appxApp{}, false},
		{hidden, appxApp{}, false},
		{"not xml", appxApp{}, false},
	}
	for i, c := range cases {
		got, ok := parseAppxManifest([]byte(c.xml))
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("case %d: got %+v, %v; want %+v, %v", i, got, ok, c.want, c.ok)
		}
	}
}

func TestResourceURIs(t *testing.T) {
	cases := map[string][]string{
		"ms-resource:AppName":                   {"ms-resource://Pkg/Resources/AppName", "ms-resource://Pkg/AppName"},
		"ms-resource:///Resources/AppName":      {"ms-resource://Pkg/Resources/AppName"},
		"ms-resource:Strings/Title":             {"ms-resource://Pkg/Strings/Title"},
		"ms-resource://Other/Resources/AppName": {"ms-resource://Other/Resources/AppName"},
	}
	for ref, want := range cases {
		if got := resourceURIs(ref, "Pkg"); !reflect.DeepEqual(got, want) {
			t.Errorf("resourceURIs(%q) = %v, want %v", ref, got, want)
		}
	}
}
