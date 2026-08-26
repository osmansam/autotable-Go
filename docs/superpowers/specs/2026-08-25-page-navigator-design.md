# Configurable Page Navigator Design

## Summary

Add a professional breadcrumb-style page navigator that tenant users configure through Page Designer. The navigator occupies a dedicated page-header region above page sections. It derives its default trail from the project's page/menu hierarchy and supports controlled overrides and manual additions.

The design follows the structural consistency of enterprise products such as Odoo and Quickbase while retaining builder flexibility. It is not a free-form grid component and does not accept arbitrary CSS or executable callbacks.

## Goals

- Give every configured page a consistent, accessible location trail.
- Generate useful navigation automatically from existing project page metadata.
- Let tenant users rename or hide generated ancestors and add controlled links.
- Avoid fragile hand-written internal paths by using page references.
- Preserve applicable runtime route parameters.
- Produce a responsive, branded component suitable for desktop and mobile.

## Non-goals

- Tenant-role restrictions. Anyone with current tenant access may configure it.
- Arbitrary JavaScript callbacks or pre-navigation submit functions.
- Arbitrary HTML or CSS entered by tenant users.
- A general-purpose menu, sidebar replacement, or navigation workflow engine.
- Record-aware breadcrumb labels in the first release.

## Placement

The navigator is page chrome, not a normal grid component. When enabled, `react-template` renders it in a dedicated page-header region after the application header and before filters and page sections. This gives all pages a stable visual hierarchy regardless of their grid layout.

Page Designer exposes the configuration under **Page Header → Breadcrumb Navigation**.

## Data Model

The page model gains an optional `pageNavigator` object. Matching types are maintained in `tenantPanel`, `react-template`, and `autotable-Go`.

```ts
type PageNavigatorMode = "automatic" | "custom";

type PageNavigatorOverride = {
  pageId: string;
  label?: string;
  hidden?: boolean;
};

type PageNavigatorDestination =
  | { type: "page"; pageId: string }
  | { type: "external"; url: string };

type PageNavigatorAdditionalItem = {
  id: string;
  label: string;
  destination: PageNavigatorDestination;
  openInNewTab?: boolean;
};

type PageNavigatorConfig = {
  enabled: boolean;
  mode: PageNavigatorMode;
  showHome: boolean;
  homeLabel?: string;
  overrides?: PageNavigatorOverride[];
  additionalItems?: PageNavigatorAdditionalItem[];
};
```

`automatic` mode starts with the generated hierarchy and applies overrides and additions. `custom` mode omits generated ancestors and renders the configured home item, manual additions, and current page. The current page is always derived from runtime context, always last, and never clickable.

New pages default to an enabled automatic navigator in the authoring UI. Existing pages without `pageNavigator` remain backward compatible and do not render a navigator until explicitly enabled. This avoids changing deployed page layouts without tenant action.

## Automatic Hierarchy

The resolver consumes the project's existing page/menu tree and current page ID. It walks from the current page to its ancestors, then reverses the result into root-to-current order.

The generated trail is:

1. Project Home, when `showHome` is enabled and a home page is available.
2. Visible ancestor pages from the page/menu hierarchy.
3. Additional configured items in their stored order.
4. The current page.

Duplicate page references are removed. The home page is not repeated when it is already an ancestor. Group-only pages may appear as labels but are clickable only when they have a valid navigable route.

Overrides match generated entries by stable page ID. A non-empty label replaces the page label. `hidden` removes an ancestor but cannot hide the current page. References to missing or deleted pages are ignored at runtime and surfaced as warnings in Page Designer.

## Navigation Semantics

Internal destinations store stable page IDs, not raw paths. At runtime, the resolver finds the current page metadata and builds the canonical tenant/project route. Applicable route parameters are carried forward when the target route declares the same parameter. Unused parameters are not appended.

External destinations accept only absolute `https:` and `http:` URLs. `javascript:`, `data:`, protocol-relative, and malformed URLs are rejected. External links may open in a new tab; new-tab links use `rel="noopener noreferrer"` and display an external-link indicator.

There are no arbitrary callbacks. Navigation uses real links so browser open-in-new-tab behavior, keyboard access, and link previews work naturally.

## Tenant Panel Experience

Page Designer adds a Page Header section with:

- Enable breadcrumb navigation.
- Mode selector: Automatic hierarchy or Custom trail.
- Show Project Home and optional home-label controls.
- Generated hierarchy preview for the selected page.
- Per-ancestor rename and hide controls.
- Additional-item list with add, remove, and drag-to-reorder actions.
- Destination selector for Project Page or External URL.
- Internal page picker populated from the current project.
- External URL validation and open-in-new-tab control.
- Desktop and mobile previews.

The preview uses the same resolution rules as the runtime, implemented through a matching pure Tenant Panel resolver and shared contract tests. It shows authoring warnings for unresolved page IDs and invalid destinations. Saving uses the existing page update flow; no new branding or navigation endpoint is introduced.

## Runtime Component

`react-template` adds a focused breadcrumb resolver and presentation component.

The resolver is a pure module that accepts page metadata, the current page, route parameters, and `PageNavigatorConfig`, and returns normalized items:

```ts
type ResolvedPageNavigatorItem = {
  id: string;
  label: string;
  href?: string;
  current: boolean;
  external: boolean;
  openInNewTab: boolean;
};
```

The React component renders semantic `<nav aria-label="Breadcrumb">` and an ordered list. Links have visible keyboard focus states. The current item uses `aria-current="page"`. Separators are decorative and hidden from assistive technology.

Visual treatment uses neutral text and borders with the project branding primary color for hover and focus states. The first item uses a restrained home icon. The component does not expose arbitrary style strings.

On narrow widths or long trails, the component retains Home, Parent, and Current and collapses intervening entries into an ellipsis menu. The menu remains keyboard accessible. Labels truncate visually but retain their full accessible name and title.

## Backend Validation and Persistence

The existing page create/update paths persist `pageNavigator`. Validation enforces:

- Known mode values.
- Bounded labels and item counts.
- Unique additional-item IDs.
- Non-empty labels for manual items.
- Exactly one destination variant per manual item.
- Valid ObjectID/page ID shape where the existing page model requires it.
- Safe absolute HTTP(S) external URLs.
- No attempt to hide or replace current-page runtime semantics.

Limits are 100 characters per label, 20 overrides, and 20 additional items. Invalid configuration returns a structured `400` response through the existing page validation path.

Page-reference existence is validated in Tenant Panel against the current project's loaded page metadata. Runtime resolution remains defensive because pages may later be removed or permissions may change.

## Error Handling

- A missing configuration renders nothing for backward compatibility.
- A disabled configuration renders nothing.
- Missing hierarchy metadata falls back to Home plus Current rather than failing the page.
- Deleted or inaccessible references are omitted.
- Invalid persisted external URLs are never rendered as links.
- An unresolved current page suppresses the navigator and does not block page content.
- Tenant Panel displays validation messages beside the relevant item and blocks invalid saves.

## Accessibility

- Semantic `nav` and ordered-list structure.
- `aria-label="Breadcrumb"` on the navigation landmark.
- `aria-current="page"` on the current entry.
- Real anchors for all clickable destinations.
- Decorative separators hidden with `aria-hidden`.
- Keyboard-operable ellipsis menu with managed focus and Escape dismissal.
- Sufficient contrast and visible focus indicators.
- Full labels available to assistive technology when visually truncated.

## Testing

### Go backend

- Page model JSON/BSON compatibility.
- Accepted automatic and custom configurations.
- Rejection of unknown modes, duplicate IDs, empty labels, excessive counts, malformed destinations, and unsafe protocols.
- Existing page documents without the field remain valid.

### Tenant Panel

- Default configuration for newly enabled navigation.
- Serialization through the existing page save flow.
- Automatic hierarchy preview.
- Rename, hide, add, remove, and reorder behavior.
- Internal page selection and external URL validation.
- Warnings for deleted page references.

### React runtime

- Automatic ancestor generation and correct order.
- Home de-duplication.
- Overrides and hidden ancestors.
- Custom mode behavior.
- Manual internal and external links.
- Deleted and invalid reference handling.
- Shared route-parameter preservation.
- Current item is last and non-clickable.
- Responsive collapsing behavior.
- Accessible landmark, current state, separators, focus behavior, and safe external-link attributes.

## Delivery Order

1. Extend and validate the backend page model.
2. Add matching Tenant Panel types, editor controls, and preview.
3. Add matching runtime types, resolver, and page-header component.
4. Run full tests and builds in all affected repositories.
