// Package navigation publishes the workspace map the signed-in person may use
// (docs/ATLAS.md §4 capability map, §6.2 permissions).
//
// Go is the authority. Every destination declares the permission its primary
// read endpoint requires, plus the exact catalogue permissions behind each
// action the destination offers. Build filters the registry with the caller's
// effective permission set, so the response never names a destination the
// caller cannot open and never offers an action they cannot perform.
//
// Hiding a link is a convenience, never a guard: each endpoint still enforces
// its own permission in the router (§6.2). The web app renders what it is given
// and maps icon names only; it does not compute access itself.
package navigation

import (
	"sort"

	"github.com/safaristack/atlas/apps/api/internal/authz"
)

// Item is one destination in the workspace map. It is the registry's shape, not
// the wire shape: Build turns it into ItemView for the caller.
type Item struct {
	// Key is a stable identifier the web app may reference in tests or
	// preferences. It never changes for a given destination.
	Key string
	// Label is the human name shown in the sidebar and breadcrumbs.
	Label string
	// Href is the web route the item opens.
	Href string
	// Icon is an icon name from the web icon set; Go does not interpret it.
	Icon string
	// Section is the breadcrumb section, matching Section.Title by convention.
	Section string
	// Keywords are extra words the quick-jump matches on.
	Keywords string
	// Badge names a permission-scoped indicator the shell may show beside the
	// item. The shell only renders a badge when it has fetched a count the
	// caller is allowed to see.
	Badge string

	// View lists the permissions required to see the destination. All of them
	// must be held. An empty list means every signed-in person sees it: the
	// destination's own endpoints are self-scoped (for example /dashboard).
	View []string

	// Actions lists the exact catalogue permission keys the destination offers
	// as buttons, links or mutations. The response reports each as a boolean so
	// the web app never keeps its own permission list.
	Actions []string
}

// Section groups destinations under an optional heading.
type Section struct {
	Key   string
	Title string
	Items []Item
}

// Registry is an ordered workspace map.
type Registry []Section

// Response is the wire shape of GET /api/v1/me/navigation.
type Response struct {
	Sections []SectionView `json:"sections"`
	// Permissions is the caller's effective permission set, sorted. Pages use
	// it for conditional reads (for example, whether to fetch approval
	// policies at all); it is the same set /api/v1/auth/me already returns.
	Permissions []string `json:"permissions"`
}

// SectionView is a visible section.
type SectionView struct {
	Key   string     `json:"key"`
	Title string     `json:"title,omitempty"`
	Items []ItemView `json:"items"`
}

// ItemView is a visible destination and the actions it offers the caller.
type ItemView struct {
	Key      string          `json:"key"`
	Label    string          `json:"label"`
	Href     string          `json:"href"`
	Icon     string          `json:"icon"`
	Section  string          `json:"section,omitempty"`
	Keywords string          `json:"keywords,omitempty"`
	Badge    string          `json:"badge,omitempty"`
	Actions  map[string]bool `json:"actions"`
}

// Build returns the destinations the principal may use, plus the effective
// permission set. Items whose View permissions are not all held are omitted
// entirely: the response never leaks the existence of a destination the caller
// cannot open. Sections left with no items are omitted too.
func (reg Registry) Build(p *authz.Principal) Response {
	out := Response{Sections: make([]SectionView, 0, len(reg)), Permissions: permissionsOf(p)}
	for _, section := range reg {
		view := SectionView{Key: section.Key, Title: section.Title, Items: make([]ItemView, 0, len(section.Items))}
		for _, item := range section.Items {
			if !canAll(p, item.View) {
				continue
			}
			view.Items = append(view.Items, item.viewFor(p))
		}
		if len(view.Items) > 0 {
			out.Sections = append(out.Sections, view)
		}
	}
	return out
}

// viewFor renders one item for the principal, reporting every declared
// permission as a boolean. The view permissions are included so a page can gate
// its own reads with the same vocabulary.
func (i Item) viewFor(p *authz.Principal) ItemView {
	keys := make([]string, 0, len(i.View)+len(i.Actions))
	keys = append(keys, i.View...)
	keys = append(keys, i.Actions...)
	actions := make(map[string]bool, len(keys))
	for _, key := range keys {
		actions[key] = p.Can(key)
	}
	return ItemView{
		Key:      i.Key,
		Label:    i.Label,
		Href:     i.Href,
		Icon:     i.Icon,
		Section:  i.Section,
		Keywords: i.Keywords,
		Badge:    i.Badge,
		Actions:  actions,
	}
}

// canAll reports whether the principal holds every named permission. A nil
// principal holds none, but an empty requirement passes (the route itself is
// authenticated).
func canAll(p *authz.Principal, required []string) bool {
	for _, permission := range required {
		if !p.Can(permission) {
			return false
		}
	}
	return true
}

// permissionsOf lists the principal's effective permissions in a stable order.
func permissionsOf(p *authz.Principal) []string {
	if p == nil {
		return []string{}
	}
	perms := make([]string, 0, len(p.Permissions))
	for permission := range p.Permissions {
		perms = append(perms, permission)
	}
	sort.Strings(perms)
	return perms
}
