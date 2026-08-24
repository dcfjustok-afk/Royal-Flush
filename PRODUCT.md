# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Stack

Vue 3 + TypeScript player and operations applications, Go modular monolith, PostgreSQL, Redis, WebSocket, LiveKit WebRTC, and Docker-based local deployment. The user delegated non-frontend stack details and approved this stack.

## Users

- Primary users are adults in mainland China who want to play a private Texas Hold'em game with existing friends while talking at the same table.
- Room owners create and moderate one private room but cannot inspect hidden cards or alter results.
- Platform operators handle user moderation, reports, audit history, and global score resets.

## Product Purpose

Royal Flush provides a dependable, invite-first online No-Limit Texas Hold'em table for friends. Success means a player can open an invite, sign in by phone, test voice, take a seat, finish a rules-correct hand, reconnect safely, and settle the session without any money or prize mechanics.

## Positioning

The product treats a private poker night as a live shared broadcast: authoritative play, visible scoring changes, and table voice are one coherent room experience rather than separate game and chat products.

## Operating Context

- Friends join by room link or room code; there is no public matchmaking.
- A room supports two to eight seated players. One account can occupy only one room at a time.
- Desktop web is the primary play surface; mobile web must support a complete hand in portrait and landscape.
- Voice is opt-in, real time, and non-recorded. Losing voice access must never block game play.

## Capabilities and Constraints

- Only friend-room No-Limit Texas Hold'em is in scope. Tournaments, bots, spectators, clubs, public chat, payments, prizes, score transfers, redemption, and withdrawal are excluded.
- New accounts begin with 1,000 account points. The balance is global, may become negative, and never gates entry to a room.
- Players may self-add 1 to 1,000,000,000 account points. An in-room addition is recorded and announced to everyone at that table, but never changes the current hand or table stack.
- Each seat session starts with 1,000 table points. Net session result is remaining table points minus all table-point allocations and is applied once to the account balance at settlement.
- Platform operators with `score:reset-all` may reset the global score epoch to a 1,000-point baseline without interrupting active games. The operation requires confirmation, a reason, and permanent audit history.
- Default chip denominations are 5, 10, 20, 50, and 100. Room owners may add only 200, 500, and 1,000 at room creation. Denominations are immutable after creation.
- Raise commands submit repeatable chip arrays. Calls remain exact, standard minimum-raise rules apply, and all-in always bypasses denomination composition.
- Phone authentication, private invite tokens, idempotent commands, authoritative server state, cryptographic shuffle, reconnect recovery, side pots, split pots, short all-ins, and heads-up blinds are required.
- The UI must always state that points are for entertainment scoring only and have no monetary value.

## Brand Commitments

- The product name is Royal Flush, with Chinese supporting copy rather than a separate Chinese brand name.
- The visual world is explicitly pinned as `深夜电台记分台`: a calm tournament scorer combined with a warm late-night radio host.
- Warm graphite, bone white, action vermilion, voice mint, and waiting amber are the committed color roles.
- Avoid casino black-and-gold styling, green felt, decorative chip piles, crowns, Las Vegas imagery, neon halos, and gambling language.
- Voice levels, broadcast messages, and precise score typography are signature brand elements.

## Evidence on Hand

No production customer claims, testimonials, commercial metrics, licensed photography, or existing brand assets are available. Demonstration players, hands, and room data must be clearly synthetic and must not imply real usage.

## Product Principles

1. Friends reach a playable private table with minimal ceremony.
2. The server is authoritative and every score-changing action is visible and auditable.
3. Voice adds presence but never becomes a prerequisite for play.
4. Account points and table points are always visually and behaviorally distinct.
5. The experience feels like a live score desk, never a casino or financial product.

## Accessibility & Inclusion

- Target WCAG 2.2 AA, including keyboard access, visible focus, reduced motion, and screen-reader labels.
- Suits use both shape and color. Game, action, speaking, network, and error states may not rely on color alone.
- Chinese is the launch language, and all layouts must tolerate longer system and recovery messages without overlap.
