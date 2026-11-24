package invoicepdf

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/go-pdf/fpdf"
)

//go:embed assets/logo.png
var logoPNG []byte

// Palette, matched to the company's existing invoice.
type rgb struct{ r, g, b int }

var (
	navy      = rgb{0x0B, 0x25, 0x7A}
	blue      = rgb{0x1F, 0x5F, 0xD0}
	skyBlue   = rgb{0x7F, 0xB6, 0xF2}
	ink       = rgb{0x14, 0x1B, 0x2D}
	muted     = rgb{0x6B, 0x72, 0x80}
	hairline  = rgb{0xD5, 0xDD, 0xEB}
	tint      = rgb{0xEE, 0xF3, 0xFB}
	danger    = rgb{0xC0, 0x1A, 0x1A}
	good      = rgb{0x15, 0x80, 0x3D}
	pageWhite = rgb{0xFF, 0xFF, 0xFF}
)

const (
	pageW   = 210.0
	pageH   = 297.0
	marginX = 17.0
	rightX  = pageW - marginX
	// bodyBottom is where content must stop to leave room for the lower
	// letterhead band and the footer.
	bodyBottom = 252.0
)

// Render draws the invoice and returns the PDF bytes.
func Render(d Document) ([]byte, error) {
	if strings.TrimSpace(d.Company.Name) == "" {
		return nil, errors.New("invoicepdf: company name is required")
	}
	if len(d.Lines) == 0 {
		return nil, errors.New("invoicepdf: at least one line is required")
	}
	if d.UnitLabel == "" {
		d.UnitLabel = "QTY"
	}

	p := fpdf.New("P", "mm", "A4", "")
	p.SetTitle("Invoice "+d.Number, true)
	p.SetAuthor(d.Company.Name, true)
	p.SetCreator("Atlas", true)
	p.SetMargins(marginX, 18, marginX)
	p.SetAutoPageBreak(false, 0)
	tr := p.UnicodeTranslatorFromDescriptor("")

	p.RegisterImageOptionsReader("logo", fpdf.ImageOptions{ImageType: "PNG", ReadDpi: false}, bytes.NewReader(logoPNG))

	r := &renderer{p: p, d: d, tr: tr}
	p.SetHeaderFuncMode(func() { r.letterhead() }, true)
	p.SetFooterFunc(func() {})
	p.AddPage()
	r.body()
	if p.Err() {
		return nil, fmt.Errorf("invoicepdf: %w", p.Error())
	}
	var out bytes.Buffer
	if err := p.Output(&out); err != nil {
		return nil, fmt.Errorf("invoicepdf: %w", err)
	}
	return out.Bytes(), nil
}

type renderer struct {
	p  *fpdf.Fpdf
	d  Document
	tr func(string) string
}

func (r *renderer) color(c rgb)  { r.p.SetTextColor(c.r, c.g, c.b) }
func (r *renderer) fill(c rgb)   { r.p.SetFillColor(c.r, c.g, c.b) }
func (r *renderer) stroke(c rgb) { r.p.SetDrawColor(c.r, c.g, c.b) }
func (r *renderer) font(style string, size float64) {
	r.p.SetFont("Helvetica", style, size)
}
func (r *renderer) text(x, y float64, s string) { r.p.Text(x, y, r.tr(s)) }
func (r *renderer) width(s string) float64      { return r.p.GetStringWidth(r.tr(s)) }

// right prints s ending at x.
func (r *renderer) right(x, y float64, s string) { r.text(x-r.width(s), y, s) }

func (r *renderer) poly(c rgb, pts ...[2]float64) {
	r.fill(c)
	ps := make([]fpdf.PointType, len(pts))
	for i, pt := range pts {
		ps[i] = fpdf.PointType{X: pt[0], Y: pt[1]}
	}
	r.p.Polygon(ps, "F")
}

// letterhead is drawn on every page: the off-white sheet, the diagonal bands
// top right and bottom left, and the SAMPLE watermark on a test copy.
func (r *renderer) letterhead() {
	p := r.p
	r.fill(pageWhite)
	p.Rect(0, 0, pageW, pageH, "F")

	// Top right: a navy wedge with two lighter bands stepping down from it.
	r.poly(sky(), [2]float64{96, 0}, [2]float64{130, 0}, [2]float64{pageW, 40}, [2]float64{pageW, 52})
	r.poly(blue, [2]float64{120, 0}, [2]float64{150, 0}, [2]float64{pageW, 26}, [2]float64{pageW, 36})
	r.poly(navy, [2]float64{140, 0}, [2]float64{pageW, 0}, [2]float64{pageW, 25})
	// A thin accent line either side of the bands.
	r.stroke(blue)
	p.SetLineWidth(0.7)
	p.Line(92, 0, 120, 16)
	p.Line(pageW-2, 31, pageW, 32.2)

	// Bottom left: the mirror image.
	r.poly(sky(), [2]float64{0, 255}, [2]float64{0, 268}, [2]float64{64, pageH}, [2]float64{44, pageH})
	r.poly(navy, [2]float64{0, 268}, [2]float64{0, pageH}, [2]float64{64, pageH})
	r.poly(blue, [2]float64{104, pageH}, [2]float64{150, pageH}, [2]float64{128, 283})
	p.SetLineWidth(0.7)
	p.Line(0, 250, 10, 256)

	if r.d.Sample {
		r.watermark("SAMPLE")
	}
}

func sky() rgb { return skyBlue }

func (r *renderer) watermark(label string) {
	p := r.p
	p.SetAlpha(0.10, "Normal")
	r.color(navy)
	r.font("B", 96)
	p.TransformBegin()
	p.TransformRotate(38, pageW/2, pageH/2)
	r.text(pageW/2-r.width(label)/2, pageH/2+10, label)
	p.TransformEnd()
	p.SetAlpha(1, "Normal")
}

func (r *renderer) body() {
	p := r.p
	d := r.d

	// ── Header: logo and company block ────────────────────────────────────
	p.ImageOptions("logo", marginX, 13, 31, 0, false, fpdf.ImageOptions{ImageType: "PNG"}, 0, "")
	x := marginX + 38
	y := 20.5
	r.color(navy)
	r.font("B", 11.5)
	r.text(x, y, d.Company.Name)
	r.color(muted)
	r.font("", 8.5)
	y += 5.6
	for _, line := range d.Company.AddressLines {
		r.text(x, y, line)
		y += 4.6
	}
	if d.Company.Phone != "" {
		r.text(x, y, d.Company.Phone)
		y += 4.6
	}
	if d.Company.Email != "" {
		r.text(x, y, d.Company.Email)
	}

	// ── Bill to ───────────────────────────────────────────────────────────
	y = 47
	r.label(marginX, y, "BILL TO")
	r.color(ink)
	r.font("B", 10.5)
	r.text(marginX, y+6, d.Client.Name)
	if d.Client.Company != "" {
		r.color(ink)
		r.font("", 9.5)
		r.text(marginX, y+11, d.Client.Company)
	}

	// ── Invoice meta, right ──────────────────────────────────────────────
	r.color(navy)
	r.font("B", 25)
	r.right(rightX, 62, "INVOICE")
	meta := [][2]string{}
	number := d.Number
	if number == "" {
		number = "DRAFT"
	}
	meta = append(meta, [2]string{"Invoice No.", number})
	meta = append(meta, [2]string{"Issue Date", d.IssueDate.Format("02/01/2006")})
	if d.DueDate != nil {
		meta = append(meta, [2]string{"Due Date", d.DueDate.Format("02/01/2006")})
	}
	if d.Status != "" {
		meta = append(meta, [2]string{"Status", d.Status})
	}
	if d.QuotationRef != "" {
		meta = append(meta, [2]string{"Quotation Reference", d.QuotationRef})
	}
	my := 70.0
	for _, m := range meta {
		r.color(muted)
		r.font("", 8.8)
		r.right(rightX-34, my, m[0])
		switch {
		case m[0] == "Status" && d.StatusTone == ToneDanger:
			r.color(danger)
		case m[0] == "Status" && d.StatusTone == ToneGood:
			r.color(good)
		default:
			r.color(ink)
		}
		r.font("B", 9)
		r.right(rightX, my, m[1])
		my += 5.4
	}

	// ── Project ───────────────────────────────────────────────────────────
	y = 66
	if d.Project.Title != "" || d.Project.Summary != "" {
		r.label(marginX, y, "PROJECT")
		r.color(ink)
		r.font("B", 10.5)
		r.text(marginX, y+6, d.Project.Title)
		if d.Project.Summary != "" {
			r.color(muted)
			r.font("", 9)
			y2 := y + 11
			for _, ln := range r.wrap(d.Project.Summary, 9, 88) {
				r.text(marginX, y2, ln)
				y2 += 4.2
			}
			y = y2 - 5 // running baseline for what follows
		} else {
			y += 7
		}
	} else {
		y += 4
	}
	if d.Project.PaymentTerms != "" {
		ty := y + 9
		r.color(muted)
		r.font("", 8.8)
		r.text(marginX, ty, "Payment Terms")
		r.color(ink)
		r.font("", 8.8)
		r.text(marginX+31, ty, d.Project.PaymentTerms)
		r.stroke(hairline)
		p.SetLineWidth(0.3)
		p.Line(marginX, ty+2, marginX+88, ty+2)
		y = ty + 4
	}

	// ── Table ─────────────────────────────────────────────────────────────
	ty := math.Max(y+6, 93)
	ty = r.table(ty)

	// ── Totals ────────────────────────────────────────────────────────────
	totals := Sum(d.Lines)
	if ty+44 > bodyBottom {
		p.AddPage()
		ty = 24
	}
	ty = r.totals(ty, totals)

	// ── Inclusions ────────────────────────────────────────────────────────
	if need := 12 + 9*float64((len(d.Inclusions)+1)/2); len(d.Inclusions) > 0 && ty+need > bodyBottom {
		p.AddPage()
		ty = 18
	}
	if len(d.Inclusions) > 0 {
		ty = r.inclusions(ty + 6)
	}
	if ty+44 > bodyBottom {
		p.AddPage()
		ty = 18
	}
	// ── Payment ───────────────────────────────────────────────────────────
	ty = r.payment(ty + 7)
	// ── Notices ───────────────────────────────────────────────────────────
	for _, n := range d.Notices {
		if strings.TrimSpace(n) == "" {
			continue
		}
		r.color(danger)
		r.font("B", 8.4)
		for _, ln := range r.wrap(n, 8.4, rightX-marginX) {
			r.text(marginX, ty+6, ln)
			ty += 4.6
		}
	}

	// ── Footer ────────────────────────────────────────────────────────────
	fy := 276.0
	r.color(navy)
	r.font("B", 9.4)
	r.right(rightX, fy, "Thank you for choosing "+d.Company.Name+".")
	r.color(muted)
	r.font("", 8.6)
	fy += 5
	if d.Company.PreparedBy != "" {
		r.right(rightX, fy, "Prepared by: "+d.Company.PreparedBy)
		fy += 4.5
	}
	if d.Company.Department != "" {
		r.right(rightX, fy, d.Company.Department+":")
		fy += 4.5
	}
	if d.Company.Email != "" {
		r.right(rightX, fy, d.Company.Email)
	}

	if d.Stamp != "" && !d.Sample {
		r.seal(96, 268, d.Stamp)
	}
}

func (r *renderer) label(x, y float64, s string) {
	r.color(blue)
	r.font("B", 8.2)
	r.spaced(x, y, s, 0.35)
}

// spaced prints s with extra space between glyphs; fpdf has no letter spacing.
func (r *renderer) spaced(x, y float64, s string, gap float64) {
	for _, ch := range s {
		c := string(ch)
		r.text(x, y, c)
		x += r.width(c) + gap
	}
}

// wrap splits s into lines no wider than maxW millimetres at the given size.
func (r *renderer) wrap(s string, size, maxW float64) []string {
	r.font("", size)
	var lines []string
	for _, para := range strings.Split(s, "\n") {
		cur := ""
		for _, w := range strings.Fields(para) {
			try := w
			if cur != "" {
				try = cur + " " + w
			}
			if r.width(try) > maxW && cur != "" {
				lines = append(lines, cur)
				cur = w
			} else {
				cur = try
			}
		}
		lines = append(lines, cur)
	}
	return lines
}

func (r *renderer) table(y float64) float64 {
	p := r.p
	d := r.d
	colQty, colRate, colAmt := 136.0, 163.0, rightX
	r.fill(tint)
	p.Rect(marginX, y, rightX-marginX, 8.6, "F")
	r.stroke(navy)
	p.SetLineWidth(0.5)
	p.Line(marginX, y+8.6, rightX, y+8.6)
	r.color(navy)
	r.font("B", 7.8)
	r.spaced(marginX+3, y+5.6, "DESCRIPTION", 0.4)
	r.right(colQty, y+5.6, strings.ToUpper(d.UnitLabel))
	r.right(colRate, y+5.6, "RATE ("+strings.ToUpper(Symbol(d.Currency))+")")
	r.right(colAmt-3, y+5.6, "AMOUNT ("+strings.ToUpper(Symbol(d.Currency))+")")
	y += 8.6

	for _, l := range d.Lines {
		if y+12 > bodyBottom {
			p.AddPage()
			y = 24
		}
		descLines := r.wrap(l.Description, 9, colQty-marginX-26)
		rowH := math.Max(7.8, 4.4*float64(len(descLines))+3.4)
		r.color(ink)
		r.font("", 9)
		ly := y + 5.3
		for _, ln := range descLines {
			r.text(marginX+3, ly, ln)
			ly += 4.4
		}
		amount := LineAmountMinor(l.Quantity, l.UnitPriceMinor)
		r.right(colQty, y+5.3, Quantity(l.Quantity))
		r.right(colRate, y+5.3, Money(l.UnitPriceMinor))
		r.right(colAmt-3, y+5.3, Money(amount))
		r.stroke(hairline)
		p.SetLineWidth(0.3)
		p.Line(marginX, y+rowH, rightX, y+rowH)
		y += rowH
	}

	t := Sum(d.Lines)
	r.fill(tint)
	p.Rect(marginX, y, rightX-marginX, 8.6, "F")
	r.stroke(hairline)
	p.Line(marginX, y+8.6, rightX, y+8.6)
	r.color(navy)
	r.font("B", 8.6)
	r.spaced(marginX+3, y+5.7, "TOTAL PAYABLE", 0.3)
	r.right(colQty, y+5.7, Quantity(t.Quantity))
	r.right(colAmt-3, y+5.7, Money(t.SubtotalMinor))
	return y + 8.6
}

func (r *renderer) totals(y float64, t Totals) float64 {
	p := r.p
	d := r.d
	left := 120.0
	row := func(label, value string, bold bool) {
		r.stroke(hairline)
		p.SetLineWidth(0.3)
		p.Line(left, y+7.6, rightX, y+7.6)
		r.color(ink)
		r.font("", 9.2)
		r.text(left+3, y+5.3, label)
		r.font("B", 9.2)
		if bold {
			r.color(ink)
		}
		r.right(rightX-3, y+5.3, value)
		y += 7.6
	}
	y += 1
	sym := Symbol(d.Currency)
	row("Subtotal", sym+" "+Money(t.SubtotalMinor), true)
	if t.TaxMinor != 0 {
		row("VAT", sym+" "+Money(t.TaxMinor), true)
	} else {
		note := d.VATNote
		if note == "" {
			note = "Not applicable"
		}
		row("VAT", note, true)
	}

	y += 2
	r.stroke(navy)
	p.SetLineWidth(0.5)
	p.Line(marginX, y, rightX, y)
	p.Line(marginX, y+12, rightX, y+12)
	r.color(navy)
	r.font("B", 9.2)
	r.spaced(marginX+3, y+7.8, "TOTAL AMOUNT DUE", 0.4)
	r.font("B", 18)
	r.right(rightX-3, y+8.8, sym+" "+Money(t.TotalMinor))

	y += 12
	r.color(ink)
	r.font("B", 8.8)
	r.text(marginX+3, y+6.5, "Amount in words:")
	labelW := r.width("Amount in words:")
	r.color(muted)
	r.font("", 8.8)
	r.text(marginX+3+labelW+2, y+6.5, AmountInWords(t.TotalMinor, d.Currency))
	return y + 7
}

func (r *renderer) inclusions(y float64) float64 {
	r.label(marginX, y, "WHAT’S INCLUDED")
	y += 2
	colW := (rightX - marginX - 8) / 2
	r.font("", 8.8)
	// Fill the left column first, then the right, balancing by item count.
	half := (len(r.d.Inclusions) + 1) / 2
	ys := [2]float64{y, y}
	for i, item := range r.d.Inclusions {
		col := 0
		if i >= half {
			col = 1
		}
		x := marginX + float64(col)*(colW+8)
		lines := r.wrap(item, 8.8, colW-5)
		r.fill(ink)
		r.p.Circle(x+1.2, ys[col]+4.1, 0.55, "F")
		r.color(ink)
		for _, ln := range lines {
			ys[col] += 4.2
			r.text(x+4, ys[col]+0.2, ln)
		}
		ys[col] += 0.9
	}
	return math.Max(ys[0], ys[1])
}

func (r *renderer) payment(y float64) float64 {
	d := r.d
	ref := d.PaymentReference
	if ref == "" {
		ref = d.Number
	}
	if ref == "" {
		ref = "Assigned on issue"
	}
	mp, bk := d.Payment.MPesa, d.Payment.Bank
	hasMPesa := mp.Number != ""
	hasBank := bk.AccountNumber != ""
	if !hasMPesa && !hasBank {
		return y
	}
	r.label(marginX, y, "PAYMENT DETAILS")
	y += 5.5
	blockW := (rightX - marginX - 12) / 2
	kv := func(x, y float64, k, v string) {
		r.color(muted)
		r.font("", 8.6)
		r.text(x, y, k)
		r.color(ink)
		r.font("", 8.8)
		r.text(x+blockW*0.47, y, v)
		r.stroke(hairline)
		r.p.SetLineWidth(0.3)
		r.p.Line(x, y+1.9, x+blockW, y+1.9)
	}
	endL, endR := y, y
	if hasMPesa {
		r.color(blue)
		r.font("B", 9.2)
		r.text(marginX, y, "M-Pesa")
		yy := y + 5.6
		kv(marginX, yy, mp.Label, mp.Number)
		yy += 5.6
		if mp.AccountRefLabel != "" {
			kv(marginX, yy, mp.AccountRefLabel, ref)
			yy += 5.6
		}
		endL = yy
	}
	if hasBank {
		x := marginX + blockW + 12
		r.color(blue)
		r.font("B", 9.2)
		r.text(x, y, "Bank Transfer")
		yy := y + 5.6
		for _, row := range [][2]string{
			{"Bank", bk.BankName}, {"Account Number", bk.AccountNumber},
			{"Account Name", bk.AccountName}, {"Payment Reference", ref},
		} {
			if row[1] == "" {
				continue
			}
			kv(x, yy, row[0], row[1])
			yy += 5.6
		}
		endR = yy
	}
	return math.Max(endL, endR)
}

// seal stamps a round company seal: two rings, the company name set round the
// top, the date across the middle and "ISSUED" beneath, tilted like a rubber
// stamp. It is drawn translucent so the text beneath stays legible.
func (r *renderer) seal(cx, cy float64, date string) {
	p := r.p
	const radius = 15.0
	p.SetAlpha(0.78, "Multiply")
	p.TransformBegin()
	p.TransformRotate(-14, cx, cy)
	r.stroke(blue)
	p.SetLineWidth(0.9)
	p.Circle(cx, cy, radius, "D")
	p.SetLineWidth(0.35)
	p.Circle(cx, cy, radius-1.5, "D")
	p.Circle(cx, cy, radius-5.8, "D")

	name := strings.ToUpper(r.d.Company.Name)
	r.color(blue)
	r.font("B", 5.6)
	// Spread the name over the upper arc, one rotated glyph at a time.
	chars := []rune(name)
	arc := math.Pi * 1.45
	rad := radius - 3.6
	start := -math.Pi/2 - arc/2
	for i, ch := range chars {
		a := start + arc*float64(i)/float64(len(chars)-1)
		x := cx + rad*math.Cos(a)
		y := cy + rad*math.Sin(a)
		p.TransformBegin()
		p.TransformRotate(-(a*180/math.Pi + 90), x, y)
		r.text(x-r.width(string(ch))/2, y+0.8, string(ch))
		p.TransformEnd()
	}

	r.font("B", 6)
	r.text(cx-r.width("ISSUED")/2, cy-1.6, "ISSUED")
	r.font("B", 6)
	r.text(cx-r.width(date)/2, cy+2.6, date)
	r.font("", 4)
	r.text(cx-r.width("OFFICIAL SEAL")/2, cy+6, "OFFICIAL SEAL")
	p.TransformEnd()
	p.SetAlpha(1, "Normal")
}
