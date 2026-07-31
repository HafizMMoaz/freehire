package collections

import (
	"reflect"
	"testing"
)

func TestParseNLSponsors_ReadsTheRegisterTable(t *testing.T) {
	// The IND register is one HTML table: the organisation is a row header, the KvK
	// number the cell beside it.
	html := `
	<table>
	  <tr><th scope="col">Organisation</th><th scope="col">KvK number</th></tr>
	  <tr><th scope="row">Adyen N.V.</th><td>34259528</td></tr>
	  <tr><th scope="row">Booking.com B.V.</th><td>31047344</td></tr>
	</table>`

	got, err := ParseNLSponsors([]byte(html))
	if err != nil {
		t.Fatalf("ParseNLSponsors: %v", err)
	}
	want := []Record{
		{Name: "Adyen N.V.", Meta: map[string]string{"kvk": "34259528"}},
		{Name: "Booking.com B.V.", Meta: map[string]string{"kvk": "31047344"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseNLSponsors =\n%+v\nwant\n%+v", got, want)
	}
}

func TestParseNLSponsors_DecodesEntitiesAndStripsMarkup(t *testing.T) {
	html := `<tr><th scope="row">Ben &amp; Jerry&#39;s <span>B.V.</span></th><td>12345678</td></tr>`
	got, err := ParseNLSponsors([]byte(html))
	if err != nil {
		t.Fatalf("ParseNLSponsors: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Ben & Jerry's B.V." {
		t.Errorf("entity/markup handling wrong: %+v", got)
	}
}

func TestParseNLSponsors_SkipsRowsWithNoOrganisation(t *testing.T) {
	html := `
	  <tr><th scope="row"></th><td>99999999</td></tr>
	  <tr><th scope="row">Adyen N.V.</th><td>34259528</td></tr>`
	got, err := ParseNLSponsors([]byte(html))
	if err != nil {
		t.Fatalf("ParseNLSponsors: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Adyen N.V." {
		t.Errorf("blank organisation not skipped: %+v", got)
	}
}

func TestParseNLSponsors_RejectsAPageWithNoRows(t *testing.T) {
	// The register page rendering without its table means the source changed; an
	// empty result would reconcile the credential off every company.
	for _, page := range []string{"", "<html><body>Service unavailable</body></html>"} {
		if _, err := ParseNLSponsors([]byte(page)); err == nil {
			t.Errorf("ParseNLSponsors accepted a page with no rows: %q", page)
		}
	}
}
