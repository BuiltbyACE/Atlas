// Package invoicepdf renders an Atlas invoice as a PDF in the company's house
// layout: letterhead, bill-to block, line-item table, totals with the amount in
// words, what is included, payment details and an issue seal.
//
// It is a pure function of a Document: no database, no network, no clock. The
// builder's live preview and the PDF that is emailed to the client both call
// Render, so what the author sees is what the client receives.
package invoicepdf

import "time"

// Document is everything the layout shows. The caller derives the status and
// notice (see StatusFor and NoticeFor) so the renderer stays dumb and testable.
type Document struct {
	Company Company
	Client  Client
	Project Project

	// Number is the invoice number, e.g. SS-INV-2026-111. Empty on a draft or a
	// sample, which prints "DRAFT".
	Number           string
	IssueDate        time.Time
	DueDate          *time.Time
	QuotationRef     string
	Status           string // human label: Draft, Issued, Overdue, Paid
	StatusTone       Tone
	Currency         string // ISO code; "KES" prints "KSh"
	UnitLabel        string // quantity column heading, e.g. PAGES, HOURS, QTY
	Lines            []Line
	VATNote          string // shown when no line carries tax, e.g. "Not applicable"
	Inclusions       []string
	Payment          Payment
	Notices          []string // red notes at the foot, e.g. the overdue notice
	PaymentReference string   // printed on the payment blocks; defaults to Number

	// Stamp, when set, applies the company seal dated with this text (the issue
	// date). Only an issued invoice carries it.
	Stamp string
	// Sample prints a diagonal SAMPLE watermark and no seal: a test copy.
	Sample bool
}

// Company is the letterhead. It lives in core.settings, not in code.
type Company struct {
	Name         string
	AddressLines []string
	Phone        string
	Email        string
	PreparedBy   string // footer line, e.g. "Safaristack Solutions Limited"
	Department   string // e.g. "Accounts Department"
}

// Client is the bill-to block.
type Client struct {
	Name    string
	Company string
}

// Project is the optional project block.
type Project struct {
	Title        string
	Summary      string
	PaymentTerms string
}

// Line is one row of the table. Quantity may be negative, which is how a
// complimentary-pages discount is expressed.
type Line struct {
	Description    string
	Quantity       float64
	UnitPriceMinor int64
	TaxRateBP      int // basis points, 1600 = 16%
}

// Payment is how the client pays. Either block may be empty.
type Payment struct {
	MPesa MPesa
	Bank  Bank
}

// MPesa is a Paybill or Till destination.
type MPesa struct {
	Label           string // "Business No. (Paybill)" or "Till No."
	Number          string
	AccountRefLabel string // "Account Reference"; empty for a Till
}

// Bank is a bank-transfer destination.
type Bank struct {
	BankName      string
	AccountNumber string
	AccountName   string
}

// Tone colours the status label.
type Tone int

const (
	ToneNeutral Tone = iota
	ToneDanger
	ToneGood
)
