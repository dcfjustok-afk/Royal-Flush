---
version: 1
slug: "apps-web-src-views-tableview-vue"
primary_target: "apps/web/src/views/TableView.vue"
related_targets: ["apps/web/src/components/ChipComposer.vue","apps/web/src/components/PlayerSeat.vue"]
---

# Table Surface Brief

## Job and mode

Operate. Two to eight existing friends need to read an authoritative Hold'em state, hear who is speaking, and submit a legal action before the timer expires. Voice presence and score transparency support the task but never compete with it.

## Approved direction

- Visual world: 深夜电台记分台.
- Non-binding decision reference: `.impeccable/mocks/table-a.png` with sidecar `.impeccable/mocks/table-a.json`; this code-led build is governed by the direction contract and implemented product truth.
- Memorable moment: one player speaks through the mint signal while the active player's vermilion timer hands control to the stable bottom dock.
- Composition: full-width station header, persistent broadcast strip, centered eight-seat oval table, and voice/chips/action zones in one fixed-height bottom dock.
- Responsive translation: desktop preserves the oval topology; mobile keeps board and pot centered, compresses remote seats around the edge, and fixes the local hand plus action dock to the bottom.

## Component grammar

- Corners: 4–6px for controls and player stations; the table alone owns the large organic oval.
- Lines: one-pixel warm-gray equipment seams; mint and vermilion replace the seam for speaking and active-turn states.
- Elevation: broad low-opacity black shadows with visible vertical offset; no colored halo or border-plus-shadow card treatment.
- Type: self-hosted Noto Sans SC for interface copy; condensed tabular numerals for pot, stack, score, and timer.
- Controls: solid rectangular command buttons, square icon tools, circular physical chip controls, and segmented controls only for exclusive room configuration.

## Sampled color record

| Role | Value | Use |
| --- | --- | --- |
| Ground | `#11110F` | Full application and table surround |
| Equipment panel | `#1D1E1B` | Dock, player station, secondary controls |
| Table surface | `#292A26` | Primary play field |
| Primary text | `#F2EFE8` | Cards, labels, high-priority values |
| Action | `#E45445` | Active timer and confirmed raise |
| Voice | `#73C6A0` | Speaking, connected, successful media state |
| Waiting | `#E0B14E` | Network and pending state only |

## Implementation inventory

| Visible ingredient | Medium | Commitment |
| --- | --- | --- |
| RF station mark | Authored semantic HTML/CSS | Compact square RF signal mark; no crown or poker-hand logo |
| Station header and broadcast | Semantic HTML/CSS + Lucide icons | Voice status centered, network/tools at right, persistent message below |
| Oval table | Responsive CSS geometry | Dominant field, wide enough for eight stable stations, no decorative card shell |
| Playing cards | Semantic HTML/CSS | Accessible rank and suit, suit shape plus color, hidden cards never rendered to unauthorized clients |
| Player stations | Reusable Vue component | Name, table points, account points, dealer, speaking, active, disconnected, empty states |
| Voice meter | CSS animated bars | One shared waveform grammar; static readable fallback under reduced motion |
| Chip controls | Semantic buttons with CSS geometry | Physical circular chips, fixed 52px desktop / 48px mobile, repeatable composition |
| Action dock | Semantic forms and buttons | Voice, chip composition, and legal action zones never resize as actions change |
| Icons | Lucide Vue | Microphone, settings, copy, undo, close, history, network, and room tools |
| Demonstration data | Typed local fixture | Clearly synthetic; replaced by authoritative snapshot when API is connected |

## States and boundaries

- Required states: loading snapshot, reconnecting, voice denied, voice disconnected, waiting for players, not the player's turn, legal action, insufficient minimum raise, all-in, showdown, settled, room ended, and global score reset.
- Account-point additions are public system events but never mutate the current table stack.
- Default chip denominations are 5, 10, 20, 50, and 100; room creation may add only 200, 500, and 1,000.
- Do not introduce public matchmaking, chat, payments, prizes, spectators, tournaments, or casino imagery.
