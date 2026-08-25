---
name: "Royal Flush"
description: "A private poker room presented as a calm late-night score broadcast."
colors:
  ground: "#0D1112"
  ground-raised: "#141A1B"
  panel: "#1B2323"
  panel-raised: "#25302F"
  table: "#263431"
  bone: "#F4F0E8"
  muted: "#B1B8B1"
  faint: "#77837D"
  seam: "#3A4945"
  seam-soft: "#293633"
  action: "#ED6A58"
  voice: "#80D3AD"
  waiting: "#EFBD63"
  focus: "#B8ECD4"
typography:
  display:
    fontFamily: "'Barlow Condensed', 'Arial Narrow', sans-serif"
    fontSize: "4rem"
    fontWeight: 700
    lineHeight: 0.8
    letterSpacing: "0"
  headline:
    fontFamily: "'Noto Sans SC', 'Microsoft YaHei UI', sans-serif"
    fontSize: "2.35rem"
    fontWeight: 700
    lineHeight: 1.18
    letterSpacing: "0"
  title:
    fontFamily: "'Noto Sans SC', 'Microsoft YaHei UI', sans-serif"
    fontSize: "1.15rem"
    fontWeight: 700
    lineHeight: 1.35
    letterSpacing: "0"
  body:
    fontFamily: "'Noto Sans SC', 'Microsoft YaHei UI', sans-serif"
    fontSize: "0.8rem"
    fontWeight: 400
    lineHeight: 1.65
    letterSpacing: "0"
  label:
    fontFamily: "'Noto Sans SC', 'Microsoft YaHei UI', sans-serif"
    fontSize: "0.68rem"
    fontWeight: 500
    lineHeight: 1.35
    letterSpacing: "0"
  measurement:
    fontFamily: "'Barlow Condensed', 'Arial Narrow', sans-serif"
    fontSize: "1rem"
    fontWeight: 600
    lineHeight: 1
    letterSpacing: "0"
rounded:
  micro: "3px"
  control: "4px"
  card: "5px"
  station: "6px"
  active-ring: "8px"
  round: "50%"
spacing:
  micro: "4px"
  xs: "8px"
  sm: "12px"
  md: "18px"
  lg: "24px"
  xl: "28px"
  section: "36px"
components:
  button-primary:
    backgroundColor: "{colors.action}"
    textColor: "#FFFFFF"
    rounded: "{rounded.control}"
    padding: "0 18px"
    height: "44px"
  button-light:
    backgroundColor: "{colors.bone}"
    textColor: "{colors.ground}"
    rounded: "{rounded.control}"
    padding: "0 18px"
    height: "44px"
  icon-tool:
    backgroundColor: "{colors.panel}"
    textColor: "{colors.bone}"
    rounded: "{rounded.control}"
    size: "38px"
  text-input:
    backgroundColor: "#111819"
    textColor: "{colors.bone}"
    rounded: "{rounded.control}"
    padding: "0 13px"
    height: "46px"
  station-nav:
    backgroundColor: "{colors.ground-raised}"
    textColor: "{colors.bone}"
    padding: "0 28px"
    height: "68px"
  system-broadcast:
    backgroundColor: "#241A19"
    textColor: "#EFD5CD"
    padding: "0 28px"
    height: "44px"
  player-station:
    backgroundColor: "#182321"
    textColor: "{colors.bone}"
    rounded: "{rounded.station}"
    padding: "8px 9px"
    width: "152px"
    height: "64px"
  physical-chip:
    backgroundColor: "#42514B"
    textColor: "#FFFFFF"
    rounded: "{rounded.round}"
    size: "52px"
  playing-card:
    backgroundColor: "{colors.bone}"
    textColor: "#171815"
    rounded: "{rounded.card}"
    padding: "7px"
    width: "54px"
    height: "74px"
  operations-metric:
    backgroundColor: "{colors.panel}"
    textColor: "{colors.bone}"
    padding: "20px"
    height: "118px"
---

# Design System: Royal Flush

## Overview

**Creative North Star: "深夜电台记分台 · 静音控制室"**

Royal Flush behaves like a calm event scorekeeper sharing a desk with a warm late-night radio host. A private poker room is presented as a live score broadcast: operational, quiet, precise, and social, with no casino glamour and no suggestion of money or prizes.

Warm graphite equipment, bone-white cards, vermilion action signals, mint voice activity, and amber waiting states make the interface feel like one coherent station. The player application and the operations console share the same restrained materials, compact controls, precise scoring figures, and visible system state; expression comes from those instruments rather than decoration.

**Key Characteristics:**
- Warm graphite equipment surfaces separated by one-pixel seams.
- Bone-white information and cards, with color reserved for operational signals.
- Condensed measurement numerals paired with highly legible Chinese interface copy.
- Compact square tools, restrained rectangular commands, circular physical chips, and one signature oval table.
- Dense, stable layouts built for repeated action and authoritative scanning.

## Colors

The palette is warm, low-chroma, and equipment-like; its three chromatic accents carry distinct operational meaning.

### Primary
- **Action Vermilion** (`#ED6A58`): confirms raises, marks the active turn, underlines active navigation, and identifies score-changing events. It is a command signal, not a general brand wash.

### Secondary
- **Voice Mint** (`#80D3AD`): identifies speaking, connected voice, healthy service state, selected media controls, and successful operations.
- **Focus Mint** (`#B8ECD4`): provides the high-contrast two-pixel keyboard focus outline without adding a glow.

### Tertiary
- **Waiting Amber** (`#EFBD63`): marks reconnecting, pending, caution, incomplete setup, and all-in availability. It never substitutes for destructive vermilion or healthy mint.

### Neutral
- **Warm Graphite** (`#0D1112`): the application ground and dominant surround.
- **Raised Graphite** (`#141A1B`): station headers, table docks, sidebars, and drawers lifted one tonal step from the ground.
- **Equipment Panel** (`#1B2323`): controls, player-adjacent tools, summary cells, and compact work surfaces.
- **Raised Equipment Panel** (`#25302F`): selected segments and secondary panel states.
- **Table Surface** (`#263431`): the authoritative play field and related table geometry.
- **Bone White** (`#F4F0E8`): primary copy, face-up cards, light command controls, and the highest-priority values.
- **Muted Copy** (`#B1B8B1`): secondary labels, explanatory copy, and inactive navigation.
- **Faint Copy** (`#77837D`): timestamps, empty states, inactive signals, and low-priority metadata.
- **Equipment Seam** (`#3A4945`): the standard one-pixel boundary between adjacent instruments.
- **Soft Seam** (`#293633`): dividers inside logs, lists, and low-emphasis groups.

**The Signal, Never Wash Rule.** Vermilion means committed or urgent action, mint means voice, connection, or success, and amber means waiting or caution; none of them becomes a decorative page tint.

## Typography

**Display Font:** Barlow Condensed (with Arial Narrow and sans-serif fallback)

**Body Font:** Noto Sans SC (with Microsoft YaHei UI and sans-serif fallback)
**Label/Measurement Font:** Noto Sans SC for labels; Barlow Condensed for measured values

**Character:** Self-hosted Noto Sans SC keeps Chinese commands and system messages neutral, steady, and readable. Barlow Condensed turns scores and game measurements into the voice of a precise live scoreboard without importing a casino display style.

### Hierarchy
- **Display measurement** (700, `4rem`, `0.8`): large account totals and other singular score readouts.
- **Headline** (700, `2.35rem`, `1.18`): major page headings; compact views step down rather than scaling fluidly with viewport width.
- **Title** (700, `1.15rem`, `1.35`): section and panel titles.
- **Body** (400, `0.8rem`, `1.65`): operational explanation, recovery messages, and narrative system copy.
- **Label** (500, `0.68rem`, `1.35`): compact metadata, field labels, navigation, and state annotations.
- **Measurement** (600, `1rem`, `1`): table points, account points, stakes, bet amounts, hand numbers, countdowns, timestamps, and other quantities.

**The Measurement Voice Rule.** Use Barlow Condensed only for quantities and compact identifiers; all commands, names, statuses, and Chinese prose remain in Noto Sans SC.

## Layout

Standard player pages use a centered `1180px` maximum content width with `24px` side clearance, reducing to `14px` side clearance below `760px`. Repeated gaps follow the observed `4px`, `8px`, `12px`, `18px`, `24px`, `28px`, and `36px` rhythm. Sections are usually unframed or full-width bands; borders and tonal fills group content without turning every section into a floating card.

The immersive table is a fixed station grid: header, broadcast, dominant stage, and stable action dock. Desktop starts at `1120px` with a `68px` header, `44px` broadcast, flexible table stage, and `218px` dock. At `620px` and below, the dock becomes a bottom control stack while the eight-seat topology remains readable; short landscape layouts up to `900px` move the dock to a `330px` side column. Standard pages stack at `760px`, and intermediate header/table compression begins at `1050px`.

The admin application is deliberately desktop-dense: a `230px` sticky sidebar, a flexible workspace, a `1120px` minimum canvas, compact tabular rows, and side drawers or dialogs for focused operational work.

**The Operational Density Rule.** Preserve scan paths, fixed control tracks, and stable dimensions before adding whitespace; this is a repeated-use score desk, not a marketing composition.

## Elevation & Depth

Depth is structural and layered. Tonal contrast and one-pixel seams establish most hierarchy; broad black shadows with visible downward offset lift only tables, stations, chips, drawers, banners, and dialogs. The core vocabulary ranges from station lift (`0 8px 22px rgba(0,0,0,.30)`) through equipment lift (`0 18px 42px rgba(0,0,0,.34)`) to the table (`0 22px 60px rgba(0,0,0,.38)`) and modal (`0 30px 80px rgba(0,0,0,.55)`).

### Shadow Vocabulary
- **Card lift** (`0 7px 14px rgba(0,0,0,.22)`): face-up playing cards only.
- **Station lift** (`0 8px 22px rgba(0,0,0,.30)`): player stations and compact state banners.
- **Equipment lift** (`0 18px 42px rgba(0,0,0,.34)`): large table-adjacent bands and waiting surfaces.
- **Table hardware** (`0 22px 60px rgba(0,0,0,.38)` plus dark inset rings): the dominant oval only.
- **Modal lift** (`0 30px 80px rgba(0,0,0,.55)`): destructive confirmation and focused admin dialogs.

**The Downward Weight Rule.** Shadows must read as physical weight beneath equipment; never replace them with colored glow, neon haze, or a border-plus-halo decoration.

## Shapes

The form language is compact and tactile. Micro badges use `3px`; buttons, inputs, icon tools, and segmented controls use `4px`; playing cards use `5px`; stations, panels, and dialogs use `6px`. An `8px` radius may appear around an active-seat progress ring so it clears the station edge. Avatars, dealer markers, toggles, event dots, and chips are circular because they represent physical or binary instruments.

The table is the only large organic silhouette. Its responsive ellipse may tighten in narrow viewports, but surrounding panels remain rectilinear and equipment-like.

**The Oval Exception Rule.** Large curvature belongs to the poker table and genuinely circular instruments; do not turn containers, navigation, commands, or status labels into pills.

## Components

### Buttons
- **Shape:** restrained rectangular controls (`4px`) with a `44px` standard command height; icon-only tools are stable `38px` squares.
- **Primary:** action vermilion with white copy, bold text, and `0 18px` horizontal padding. Use it for the single committed action in a control group.
- **Light:** bone white on warm graphite for the authoritative neutral action such as call or check.
- **Hover / Focus:** hover may brighten slightly; keyboard focus uses a two-pixel Focus Mint outline with a `3px` offset. Disabled controls become low-contrast graphite and lose shadow.
- **Danger / All-in:** destructive admin commands use a dark vermilion surface and border; all-in uses an amber-bordered dark surface. Neither impersonates the primary action.

### Chips
- **Style:** physical circular controls at `52px` desktop, `46–48px` compact portrait, and `42px` short landscape. A dashed bone edge, two inset rings, and a small downward shadow make them tactile.
- **State:** denomination colors are muted equipment tones. Hover lifts by `3px`; active returns to rest; disabled chips remain in place at reduced opacity.

### Cards / Containers
- **Corner Style:** cards use `5px`; stations and equipment panels use `6px`.
- **Background:** face-up cards are Bone White; equipment containers step through Raised Graphite, Equipment Panel, and Table Surface.
- **Shadow Strategy:** only physically lifted objects receive shadows; page sections rely on tonal layering and seams.
- **Border:** use a one-pixel Equipment Seam, replacing it with Voice Mint for speaking and an external vermilion progress ring for the current actor.
- **Internal Padding:** compact stations use `8px 9px`; larger panels generally use `20–24px`.

### Inputs / Fields
- **Style:** a dark recessed field (`#111819`), one-pixel Equipment Seam, `6px` corners, `46px` minimum height, and Bone White text.
- **Focus:** border and one-pixel inner outline shift to Voice Mint; the global Focus Mint keyboard outline remains visible where applicable.
- **Error / Disabled:** errors use softened vermilion copy and explicit text; disabled fields and controls reduce contrast without disappearing.

### Navigation
- **Style:** headers and sidebars are station hardware, separated from content by one-pixel seams. Links are compact muted labels; the active web route gains Bone White copy and a two-pixel vermilion underline, while the active admin route uses an Equipment Panel fill and a mint icon.
- **Mobile:** nonessential labels collapse below `760px`, but the RF station mark and primary account or table tools retain fixed hit areas.

### Player Station

Each station is a stable equipment readout for name, table points, account points, dealer, voice, turn, and connection state. Desktop stations are `152px` wide with a `64px` minimum height; mobile compresses labels and icons while preserving the two score columns. Speaking replaces the seam with mint, active play gains a vermilion progress ring, and disconnected state also reduces opacity so status never relies on color alone.

### System Broadcast

The broadcast is a persistent full-width strip with a dark vermilion-brown surface, explicit source label, message, and condensed timestamp. It announces score changes and authoritative system events without becoming an alert card or overlay.

### Eight-seat Table and Action Dock

The dominant table uses a responsive ellipse, inset equipment rings, centered pot and board, and stable positions for up to eight player stations. The dock separates voice, repeatable chip composition, action summary, legal commands, and countdown into fixed tracks; changing legal actions must not resize the dock.

**The Stable Command Rule.** Preserve the dimensions and order of score, chip, voice, timer, and legal-action controls across state changes so urgency never causes layout shift.

## Do's and Don'ts

### Do:
- **Do** use Warm Graphite as the ground and layer equipment with tonal steps, one-pixel seams, and restrained downward shadows.
- **Do** reserve Action Vermilion for committed or urgent action, Voice Mint for voice, connection, or success, and Waiting Amber for waiting or caution.
- **Do** use Noto Sans SC for Chinese interface copy and Barlow Condensed for points, stakes, bets, hand numbers, countdowns, timestamps, and other measurements.
- **Do** keep controls compact, tactile, keyboard-visible, and dimensionally stable across loading, reconnecting, disabled, success, and error states.
- **Do** pair every color signal with shape, icon, text, border, position, or opacity, and honor reduced-motion preferences.

### Don't:
- **Don't** introduce black-and-gold casino styling, green felt, chip-pile decoration, crowns, Las Vegas photography, neon haze, or gambling and money metaphors.
- **Don't** use decorative cards as page scaffolds, nest cards inside cards, or float every section in a rounded container.
- **Don't** turn controls, navigation, or status labels into a pill-heavy interface; reserve circles and the large oval for physical instruments and the table.
- **Don't** use colored glow, blur, or halo as elevation; depth comes from tonal contrast, seams, and broad black shadows with visible offset.
- **Don't** let voice, animation, decorative imagery, or responsive compression obscure authoritative game state or block legal play.
