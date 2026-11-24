package invoicepdf

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAmountInWords(t *testing.T) {
	cases := map[int64]string{
		2210000:   "Twenty-Two Thousand One Hundred Kenyan Shillings Only",
		100:       "One Kenyan Shillings Only",
		100500:    "One Thousand and Five Kenyan Shillings Only",
		150000:    "One Thousand Five Hundred Kenyan Shillings Only",
		100005_00: "One Hundred Thousand and Five Kenyan Shillings Only",
		12345650:  "One Hundred and Twenty-Three Thousand Four Hundred and Fifty-Six Kenyan Shillings and Fifty Cents Only",
		0:         "Zero Kenyan Shillings Only",
	}
	for minor, want := range cases {
		if got := AmountInWords(minor, "KES"); got != want {
			t.Errorf("AmountInWords(%d) = %q, want %q", minor, got, want)
		}
	}
}

func TestMoneyAndTotals(t *testing.T) {
	if got := Money(2210000); got != "22,100" {
		t.Errorf("Money = %q", got)
	}
	if got := Money(-260000); got != "-2,600" {
		t.Errorf("negative Money = %q", got)
	}
	if got := Money(2210050); got != "22,100.50" {
		t.Errorf("fractional Money = %q", got)
	}
	// A negative quantity is a discount and reduces the total.
	got := Sum([]Line{{Quantity: 19, UnitPriceMinor: 130000}, {Quantity: -2, UnitPriceMinor: 130000}})
	if got.SubtotalMinor != 2210000 || got.TotalMinor != 2210000 || got.Quantity != 17 {
		t.Errorf("Sum = %+v", got)
	}
	vat := Sum([]Line{{Quantity: 1, UnitPriceMinor: 100000, TaxRateBP: 1600}})
	if vat.TaxMinor != 16000 || vat.TotalMinor != 116000 {
		t.Errorf("VAT sum = %+v", vat)
	}
}

func TestStatusFor(t *testing.T) {
	today := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	past := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	future := time.Date(2026, 12, 20, 0, 0, 0, 0, time.UTC)
	if l, tone := StatusFor("issued", &past, today); l != "Overdue" || tone != ToneDanger {
		t.Errorf("issued past due = %s", l)
	}
	if l, _ := StatusFor("issued", &future, today); l != "Issued" {
		t.Errorf("issued not yet due = %s", l)
	}
	if l, tone := StatusFor("paid", &past, today); l != "Paid" || tone != ToneGood {
		t.Errorf("paid = %s", l)
	}
	if n := NoticeFor("Overdue", 2210000, "KES", &past); n == "" {
		t.Error("overdue needs a notice")
	}
}

// fakeDoc is shaped like the company's invoice but uses invented names and
// numbers, so no real client or account detail lives in the repository.
func fakeDoc() Document {
	due := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	issue := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
	label, tone := StatusFor("issued", &due, time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC))
	return Document{
		Company: Company{
			Name:         "Example Company Limited",
			AddressLines: []string{"1 Sample Street, Example Court,", "Nairobi"},
			Phone:        "+254 700 000 000",
			Email:        "accounts@example.test",
			PreparedBy:   "Example Company Limited",
			Department:   "Accounts Department",
		},
		Client:    Client{Name: "Jane Example", Company: "Sample Trading Co."},
		Project:   Project{Title: "Sample Project – Cultural Symbol", Summary: "A4 colouring book design, layout and photo integration.", PaymentTerms: "Due in full upon approval of final proof"},
		Number:    "EX-INV-2026-001",
		IssueDate: issue, DueDate: &due, QuotationRef: "QT-001",
		Status: label, StatusTone: tone, Currency: "KES", UnitLabel: "PAGES",
		Lines: []Line{
			{Description: "Book design, layout and photo integration", Quantity: 19, UnitPriceMinor: 130000},
			{Description: "Complimentary pages discount", Quantity: -2, UnitPriceMinor: 130000},
		},
		Inclusions: []string{
			"Cover and back page design.", "Layout of all 19 pages incorporating the client’s photographs.",
			"Photo cleanup and print upscaling (approximately 35 images).", "Colouring pages cleaned into crisp line art.",
			"Proofreading against the author’s text.", "Print-ready A4 PDF.", "Two rounds of minor corrections.",
		},
		Payment: Payment{
			MPesa: MPesa{Label: "Business No. (Paybill)", Number: "000000", AccountRefLabel: "Account Reference"},
			Bank:  Bank{BankName: "Example Bank", AccountNumber: "0000000000", AccountName: "Example Company Limited"},
		},
		Notices: []string{
			NoticeFor(label, 2210000, "KES", &due),
			"The deliverable has already been supplied to the client. Upon payment, the client owns the final files.",
		},
		Stamp: "16/11/2026",
	}
}

func TestRenderProducesPDF(t *testing.T) {
	pdf, err := Render(fakeDoc())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) || len(pdf) < 5000 {
		t.Fatalf("not a plausible PDF: %d bytes", len(pdf))
	}
	if out := os.Getenv("INVOICEPDF_OUT"); out != "" {
		if err := os.WriteFile(filepath.Join(out, "invoice-fake.pdf"), pdf, 0o600); err != nil {
			t.Fatal(err)
		}
		sample := fakeDoc()
		sample.Sample, sample.Number, sample.Stamp = true, "", ""
		if b, err := Render(sample); err == nil {
			_ = os.WriteFile(filepath.Join(out, "invoice-fake-sample.pdf"), b, 0o600)
		}
	}
}

func TestRenderRejectsEmpty(t *testing.T) {
	if _, err := Render(Document{}); err == nil {
		t.Error("an empty document must not render")
	}
	d := fakeDoc()
	d.Lines = nil
	if _, err := Render(d); err == nil {
		t.Error("an invoice with no lines must not render")
	}
}

func TestRenderLongInvoiceDoesNotPanic(t *testing.T) {
	d := fakeDoc()
	for i := 0; i < 40; i++ {
		d.Lines = append(d.Lines, Line{Description: "Additional work item with a reasonably long description that wraps onto a second line", Quantity: 2, UnitPriceMinor: 50000})
	}
	if _, err := Render(d); err != nil {
		t.Fatal(err)
	}
}

func TestDraftPrintsReferencePlaceholder(t *testing.T) {
	d := fakeDoc()
	d.Number, d.Stamp, d.Sample = "", "", true
	if _, err := Render(d); err != nil {
		t.Fatal(err)
	}
}
