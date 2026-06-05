# 🏗️ Architecture & Data Flow Diagrams

## System Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                        CLIENT (Browser)                             │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │ HTML Templates + Tailwind CSS + HTMX (NO React!)           │   │
│  │ - home.html, product.html, checkout.html, payment.html     │   │
│  │ - user_panel.html, admin_panel.html, branding.html         │   │
│  │ - calendar.html (HTMX fragment)                            ��   │
│  └─────────────────────────────────────────────────────────────┘   │
└────────────────────────────────────────────────────────────────────┬┘
                                 │
                    HTTP (GET/POST with HTMX)
                                 │
┌────────────────────────────────────────────────────────────────────┬┘
│                         HTTP HANDLERS                              │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │ cmd/server/server.go + internal/handlers/*.go              │   │
│  │                                                             │   │
│  │ Routes:                                                     │   │
│  │ • Home, Product, Calendar (handleHome, handleProduct...)  │   │
│  │ • Cart (handleCartAdd, handleCartRemove)                  │   │
│  │ • Checkout (handleCheckout, handleCheckoutSubmit)         │   │
│  │ • Payment (handlePayment, handlePaymentStatus)            │   │
│  │ • User (handleUserPanel, handleUserDelete*)              │   │
│  │ • Admin (handleAdminPanel, handleAdminOrder*, ...)       │   │
│  │ • Branding (handleBrandingLibrary)                        │   │
│  └─────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────��──────────────┘
                                 │
        ┌────────────────────────┼────────────────────────┐
        │                        │                        │
        ▼                        ▼                        ▼
┌──────────────┐      ┌──────────────┐      ┌──────────────┐
│ Event Store  │      │ Product      │      │ Auth Manager │
│              │      │ Parser       │      │              │
│ emit events  │      │              │      │ Login/Logout │
│              │      │ Load from    │      │ Sessions     │
│              │      │ .md files    │      │ Passwords    │
└──────────────┘      └──────────────┘      └──────────────┘
        │
        │ Events: OrderPlaced, OrderPaid,
        │         UserRegistered, TransferReceived, etc.
        │
        ▼
┌��─────────────────────────────────────────────────────────┐
│          EVENT STORE (event_store.db - SQLite)          │
├──────────────────────────────���───────────────────────────┤
│ ┌──────────────────────────────────┐                    │
│ │ events (append-only log)         │                    │
│ │  - event_id, aggregate_id        │                    │
│ │  - event_type, payload (JSON)    │                    │
│ │  - created_at                    │                    │
│ └──────────────────────────────────┘                    │
│ ┌──────────────────────────────────┐                    │
│ │ event_snapshots (perf optimization)                  │
│ │  - aggregate_id, version         │                    │
│ │  - snapshot_data (JSON)          │                    │
│ └──────────────────────────────────┘                    │
└──────────────────────────────────────────────────────────┘
        │
        │ Read/subscribe to events
        │
        ▼
┌──────────────────────────────────────────────────────────┐
│     PROJECTIONS / PROJECTOR (Update Read Models)         │
├───────────────────────────────────────────���──────────────┤
│ • OrderPlacedProjector → orders, order_items tables     │
│ • OrderPaidProjector → update orders.status             │
│ • UserRegisteredProjector → users table                 │
│ • TransferReceivedProjector → transfers table           │
│ • TransferLinkedProjector → update transfers.status     │
└──────────────────────────────────────────────────────────┘
        │
        ▼
┌──────────────────────────────────────────────────────────┐
│    READ MODELS (read_models.db - SQLite Denormalized)   │
├──────────────────────────────────────────────────────────┤
│ ┌──────────────────┐  ┌──────────────────┐              │
│ │ users            │  │ orders           │              │
│ │ - id, email      │  │ - id (CARGO-xxx) │              │
│ │ - password_hash  │  │ - user_id        │              │
│ │ - name, phone    │  │ - status         │              │
│ │ - address        │  │ - total_amount   │              │
│ │ - is_adult       │  │ - payment_method │              │
│ │ - accepted_tos   │  │ - created_at     │              │
│ └──────────────────┘  └──────────────────┘              │
│ ┌──────────────────┐  ┌──────────────────┐              │
│ │ order_items      │  │ transfers        │              │
│ │ - order_id       │  │ - id             │              │
│ │ - product_id     │  │ - sender_name    │              │
│ │ - base_price     │  │ - amount         │              │
│ │ - quantity_days  │  │ - order_title    │              │
│ │ - item_total     │  │ - status         │              │
│ └──────────────────┘  └──────────────────┘              │
│ ┌──────────────────┐  ┌──────────────────┐              │
│ │ product_bookings │  │ user_sessions    │              │
│ │ - product_id     │  │ - id (token)     │              │
│ │ - booked_date    │  │ - user_id        │              │
│ │ - order_id       │  │ - expires_at     │              │
│ └──────────────────┘  └──────────────────┘              │
└──────────────────────────────────────────────────────────┘
        │
        │ Query for HTTP responses
        │
        └──────────────────┬───���─────────────┐
                           │                 │
        ┌──────────────────┴──────┐  ┌──────┴──────┐
        │   User Queries          │  │Admin Queries│
        │  (orders, sessions)     │  │ (all data)  │
        │                         │  │             │
        ▼                         ▼  ▼             ▼
   ┌─────────┐              ┌──────────┐
   │ Session │              │ Admin    │
   │ Mgmt    │              │ Reports  │
   └─────────┘              └──────────┘
```

---

## User Journey: Home → Product → Cart → Checkout → Payment → Success

```
START
  │
  ├─► GET /
  │   ├─ Query products from /data/products/*.md
  │   ├─ Render home.html with product grid
  │   └─ Show cart badge (0) in header
  │
  ├─► GET /product/{id}
  │   ├─ Load product details
  │   ├─ Query product_bookings for booked dates
  │   ├─ Render product.html with calendar
  │   └─ Calculate total: basePrice * days + addons
  │
  ├─► GET /product/{id}/calendar?month=6&year=2026
  │   ├─ HTMX request (calendar navigation)
  │   ├─ Return calendar.html fragment
  │   └─ Update calendar display
  │
  ├─► POST /cart/add
  │   ├─ Parse: product_id, start_date, end_date, selected_addons[]
  │   ├─ Validate dates & availability
  │   ├─ Get/create session ID
  │   ├─ Store in shopping_carts table (or session cookie)
  │   ├─ JSON serialize items
  │   ├─ Return HTMX fragment: updated cart badge
  │   └─ Redirect to /checkout
  │
  ├─► GET /checkout
  │   ├─ Query shopping_carts for items
  │   ├─ Enrich with product details
  │   ├─ Show form: name, email, phone, address, password (if new)
  │   ├─ Show cart summary on right (sticky)
  │   ├─ Show payment method selection (BLIK or Cash)
  │   └─ Render checkout.html
  │
  │   [User fills form & checks boxes]
  │
  ├─► POST /checkout/submit
  │   ├─ Validate form (server-side):
  │   │  ├─ Check name, email, phone, address
  │   │  ├─ Check password strength (if new user)
  │   │  ├─ Check isAdult = TRUE
  │   │  └─ Check acceptTos = TRUE
  │   ├─ Register user OR verify existing:
  │   │  ├─ If new: Hash password, insert to users table
  │   │  ├─ **Emit UserRegistered event**
  │   │  └─ Create session token
  │   ├─ **Emit OrderPlaced event** with order details
  │   │  ├─ Generate order_id: "CARGO-{random}"
  │   │  ├─ Include rental_items[], total, payment_method
  │   │  └─ Include user_id, timestamp
  │   ├─ PROJECTOR processes OrderPlaced:
  │   │  ├─ Insert into orders table
  │   │  ├─ Insert into order_items table
  │   │  ├─ For each rental day: insert into product_bookings
  │   │  └─ Update product_bookings query results (booked dates)
  │   ├─ Clear shopping_carts for this session
  │   ├─ Set auth session cookie
  │   └─ Redirect to /payment/{orderID}
  │
  ├─► GET /payment/{orderID}
  │   ├─ Query orders table: GET status, payment_method, amount
  │   ├─ If payment_method = "blik":
  │   │  ├─ Show BLIK instructions page
  │   │  ├─ Generate order title: "CARGO-{4 random digits}"
  │   │  ├─ Client sees: phone number, amount, order title
  │   │  ├─ Button "Wysłałem przelew BLIK"
  │   │  └─ On click: Show loader
  │   ├─ If payment_method = "cash":
  │   │  ├─ Show cash instructions
  │   │  ├─ Button "Rozumiem, sfinalizuj rezerwację"
  │   │  └─ On click: **Emit OrderPaid event** → redirect /success
  │   └─ Render payment.html
  │
  ├─ HTMX POLLING (for BLIK):
  │  ├─► GET /payment/{orderID}/status (triggered every 3 seconds)
  │  │   └─ Query orders.status
  │  ├─ If status = "pending":
  │  │  └─ Return empty fragment (no change, keep polling)
  │  └─ If status = "paid":
  │     ├─ Return success fragment
  │     ├─ HTMX swaps outerHTML (replaces loader)
  │     └─ Remove hx-trigger (stops polling)
  │
  ├─ [BACKGROUND] BLIK EMAIL WORKER:
  │  ├─ Every 30 seconds:
  │  │  ├─ Connect to IMAP, fetch unread emails
  │  │  ├─ Parse subject line: "CARGO-1234"
  │  │  ├─ **Emit TransferReceived event**
  │  │  ├─ PROJECTOR inserts to transfers table (status = "unmatched")
  │  │  ├─ Worker queries orders matching CARGO-1234
  │  │  ├─ If amount matches exactly:
  │  │  │  ├─ **Emit TransferLinked event**
  │  │  │  ├─ **Emit OrderPaid event**
  │  │  │  └─ PROJECTOR updates: orders.status = "paid", orders.paid_at = NOW
  │  │  ├─ Email polling loop continues
  │  │  └─ HTMX polling detects status change → shows success
  │  └─ Mark email as read on server
  │
  ├─► GET /success (or served via HTMX fragment)
  │   ├─ Query orders for details
  │   ├─ Show: "Udało się! Do zobaczenia w Radzyminie."
  │   ├─ Show confirmation email message
  │   ├─ Button "Przejdź do Twojego Panelu Klienta" → /user
  │   └─ Render success.html
  │
  └─ END

```

---

## Admin Journey: View Orders → Link Transfer → Mark Paid

```
START
  │
  ├─► GET /admin (with admin session verified)
  │   ├─ Query orders table (recent, limit 10)
  │   ├─ Query transfers table (unmatched first)
  │   ├─ Query user info for display
  │   ├─ Render admin_panel.html with two tables
  │   └─ Show "Last sync: X min ago"
  │
  │   [ORDERS TABLE - Shows pending orders]
  │   
  ���─► POST /admin/order/{orderID}/mark-paid
  │   ├─ HTMX POST (from button on order row)
  │   ├─ Verify admin session (middleware)
  │   ├─ Validate order exists
  │   ├─ **Emit OrderPaid event** with method="manual"
  │   ├─ PROJECTOR updates: orders.status = "paid"
  │   ├─ Return updated order row (HTMX fragment)
  │   └─ Browser updates table (no page reload)
  │
  │   [TRANSFERS TABLE - Shows unmatched transfers]
  │
  ├─► Form: Link Transfer to Order
  │   ├─ Show dropdown: list of pending orders
  │   ├─ Admin selects matching order
  │   ├─ Click "Czasem" (Link button)
  │   └─ POST /admin/transfer/{transferID}/link
  │
  ├─► POST /admin/transfer/{transferID}/link
  │   ├─ HTMX POST
  │   ├─ Get transfer from transfers table
  │   ├─ Get order from orders table
  │   ├─ Verify amounts match exactly
  │   ├─ If mismatch: Return error message (HTMX fragment)
  │   ├─ If match:
  │   │  ├─ **Emit TransferLinked event**
  │   │  ├─ **Emit OrderPaid event**
  │   │  ├─ PROJECTOR updates: transfers.status = "linked"
  │   │  ├─ PROJECTOR updates: orders.status = "paid"
  │   │  └─ Return updated transfers section (HTMX fragment)
  │   └─ Transfer disappears from unmatched list
  │
  └─ END

```

---

## Event-Sourcing Flow: How Events Create Read Models

```
┌─────────────────────────────────────────┐
│    COMMAND (User Action)                │
│  "PlaceOrder" / "RegisterUser" / etc    │
└────────────┬────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────────────────────┐
│    HANDLER (validates & emits event)                    │
│  • Validate business rules               │
│  • Generate unique IDs                   │
│  • Create event payload (JSON)           │
└────────────┬────────────────────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────────────────────┐
│    EVENT STORE (store.go)                               │
│  • Append event to events table (SQL)     │
│  • Log: aggregate_id, event_type, payload│
│  • Timestamp: CURRENT_TIMESTAMP          │
│  • Index by aggregate_id for queries     │
└────────────┬────────────────────────────────────────────���
             │
             ▼ (can be consumed by multiple projectors)
          ┌──┴──────────────────────────┬─────────────┐
          │                             │             │
          ▼                             ▼             ▼
    ┌─────────────┐             ┌─────────────┐  ┌──────────┐
    │OrderPlaced  │             │UserRegist   │  │Transfer  │
    │Projector    │             │Projector    │  │Projector │
    └────┬────────┘             └────┬────────┘  └────┬─────┘
         │                           │               │
         ├─ Read event payload       │               │
         ├─ Validate               │               │
         ├─ Extract data           │               │
         │                         │               │
         ▼                         ▼               ▼
    ┌──────────────────┐  ┌──────────────┐  ┌─────────���──┐
    │INSERT orders     │  │INSERT users  │  │INSERT      │
    │INSERT order_items│  │table row     │  │transfers   │
    │INSERT booking    │  │              │  │table row   │
    │dates into        │  │              │  │            │
    │product_bookings  │  │              │  │            │
    └──────────────────┘  └──────────────┘  └────────────┘
         │                    │                    │
         └────────┬───────────┴────────────────────┘
                  │
                  ▼ (Read Models are now consistent)
         ┌────────────────────────┐
         │ read_models.db (SQLite)│
         │ • users               │
         │ • orders              │
         │ • order_items         │
         │ • transfers           │
         │ • product_bookings    │
         │ • sessions            │
         └────────────────────────┘
                  │
                  │ HTTP Handlers query these tables
                  │
                  ► Web UI renders up-to-date data
```

---

## Session & Authentication Flow

```
┌─────────────────────┐
│  User enters email  │
│   + password        │
└──────────┬──────────┘
           │
           ▼
    ┌─────────────────────────────────┐
    │POST /login                      │
    │  • Fetch user from users table  │
    │  • Compare password with hash   │
    │  • If mismatch: return error    │
    └──────────┬──────────────────────┘
               │
               ▼ (if match)
    ┌─────────────────────────────────┐
    │Generate session token (UUID)    │
    │Insert into user_sessions table: │
    │  • id (token)                   │
    │  • user_id                      │
    │  • is_admin (from users table)  │
    │  • created_at                   │
    │  • expires_at (+ 30 days)       │
    └──────────┬──────────────────────┘
               │
               ▼
    ┌─────────────────────────────────┐
    │Set HttpOnly Secure cookie:      │
    │  Name: session_token            │
    │  Value: <token>                 │
    │  Path: /                        │
    │  HttpOnly: true                 │
    │  Secure: true                   │
    │  SameSite: Lax                  │
    │  Max-Age: 30 days               │
    └──────────┬──────────────────────┘
               │
               ▼ (cookie stored in browser)
    ┌─────────────────────────────────┐
    │Redirect to /user                │
    │                                 │
    │(Every subsequent request adds   │
    │ cookie to request headers)      │
    └─────────────────────────────────┘

[Session Middleware runs on protected routes]

    ┌─────────────────────────────────┐
    │Extract session cookie from      │
    │request headers                  │
    └──────────┬──────────────────────┘
               │
               ▼
    ┌─────────────────────────────────┐
    │Query user_sessions table        │
    │WHERE id = <cookie_value>        │
    └──────────┬──────────────────────┘
               │
               ▼
    ┌─────────────────────────────────┐
    │Check expires_at > NOW           │
    │  If expired: return 401         │
    │  If valid: continue             │
    └──────────┬──────────────────────┘
               │
               ▼
    ┌───────────────────────────────���─┐
    │Update last_activity = NOW       │
    │                                 │
    │Add user to request context      │
    │(ctx.Value("user") = user data)  │
    └──────────┬──────────────────────┘
               │
               ▼
    ┌──────────────────────��──────────┐
    │Handler has access to:           │
    │  • user.ID                      │
    │  • user.Email                   │
    │  • user.is_admin                │
    └─────────────────────────────────┘
```

---

## Email Payment Worker Cycle

```
                    ┌──────────────────────────┐
                    │  BLIK Email Worker       │
                    │  (runs in goroutine)     │
                    └────────────┬─────────────┘
                                 │
                    ┌────────────┴────────────┐
                    │ Start: Every 30 seconds │
                    └────────────┬────────────┘
                                 │
                    ┌────────────▼──────────────┐
                    │ 1. Connect to IMAP       │
                    │    (Outlook server)      │
                    └────────────┬──────────────┘
                                 │
                    ┌────────────▼──────────────┐
                    │ 2. Fetch unread emails   │
                    └────────────┬──────────────┘
                                 │
                    ┌────────────▼──────────────┐
                    │ 3. Filter subject line   │
                    │    "CARGO-*" pattern     │
                    │    Extract: CARGO-1234   │
                    └────────────┬──────────────┘
                                 │
                    ┌────────────▼──────────────┐
         ┌──────────┤ 4. Parse email body      │
         │          │    Extract amount        │
         │          │    Extract from/sender   │
         │          ��───────────────────────────┘
         │                     │
         │          ┌──────────▼──────────────┐
         │          │ 5. **Emit event**:      │
         │          │    TransferReceived     │
         │          │    ├─ sender_name       │
         │          │    ├─ amount            │
         │          │    ├─ order_title       │
         │          │    └─ timestamp         │
         │          └──────────┬──────────────┘
         │                     │
         │          ┌──────────▼──────────────┐
         │          │ PROJECTOR processes:    │
         │          │                         │
         │          │ INSERT transfers table: │
         │          │  ├─ id (UUID)           │
         │          │  ├─ sender_name         │
         │          │  ├─ amount              │
         │          │  ├─ order_title         │
         │          │  ├─ status="unmatched"  │
         │          │  └─ received_at=NOW     │
         │          └──────────┬──────────────┘
         │                     │
         │          ┌──────────▼──────────────┐
         │          │ 6. Query orders table   │
         │          │    WHERE order_id =     │
         │          │    "CARGO-1234"         │
         │          └──────────┬──────────────┘
         │                     │
         └─────────────────────┤
                               │
                    ┌──────────▼────────────────┐
                    │ 7. Amount Validation      │
                    └────────────┬──────────────┘
                                 │
            ┌────────────┬────────┴──────────┬─────────┐
            │            │                   │         │
        No Order    Amount∙Mismatch    Order Exists  Perfect Match
            │            │               (Old order) │
            ├─ Ignore    ├─ Mark trans     │       ├─ **Emit**:
            │  (leave    │  status         └─ Ignore │ TransferLinked
            │  unmatched)│  "mismatch"     │       │
            │            │                 │       ├─ **Emit**:
            │            │                 │       │ OrderPaid
            │            │                 │       │
            │            │                 │       ├─ PROJECTOR
            │            │                 │       │ updates:
            │            │                 │       │  • transfers
            │            │                 │       │    status=linked
            │            │                 │       │  • orders
            │            │                 │       │    status=paid
            │            │                 │       │    paid_at=NOW
            └────────┬───┴────┬────────────┴───────┘
                     │        │
                     └─┬──────┘
                       │
         ┌────────────▼──────────────┐
         │ 8. Mark email as read     │
         │    on IMAP server         │
         └──────────────────────────┘
                     │
         ┌───────────▼──────────────┐
         │ 9. Sleep 30 seconds      │
         └────────────┬─────────────┘
                      │
                 Go to #1
```

---

## Data Dependency Chart

```
┌─ SHOPPING CART (in cookie or session)
│   ├─ CartItem[]
│   │  ├─ product_id ──┐
│   │  ├─ start_date   │
│   │  └─ end_date     ├─► product_id must exist in /data/products/*.md
│   │                  │
│   └─ User session ID ├─► stored in user_sessions (if logged in)
│
├─ CHECKOUT FORM
│   ├─ User Details ────────────► users table (if registering)
│   ├─ Order Data ──────────────► orders table (after event)
│   └─ Cart Items ──────────────► order_items table (after event)
│
├─ PAYMENT STATUS POLLING
│   └─ order_id ────────────────► orders.status (polls this)
│
├─ PRODUCT CALENDAR
│   ├─ product_id ──────────────► product_bookings table (booked dates)
│   └─ year/month ──────────────► generateCalendarGrid() logic
│
├─ ADMIN PANEL
│   ├─ orders table ────────────► JOIN with users table
│   ├─ transfers table ─────────► links to orders table
│   └─ user_sessions ───────────► is_admin flag check
│
└─ USER PANEL
    ├─ user_id ────────────────► users table
    └─ user_id ────────────────► orders table (where user_id = ?)
```

---

## Database Relationship Diagram

```
users (1)
  │ ├─ id (PK)
  │ ├─ email (UNIQUE)
  │ ├─ password_hash
  │ ├─ name, phone, address
  │ └─ is_adult, accepted_tos
  │
  ├──┬─────────────────────────────────────────────┐
  │  │                                             │
  │  ▼ (user_id)                               (user_id)
orders (*)                                  user_sessions (*)
  ├─ id (PK) "CARGO-XXXX"                      ├─ id (PK) token
  ├─ user_id (FK)                              ├─ user_id (FK)
  ├─ status                                     ├─ is_admin
  ├─ payment_method                             ├─ expires_at
  ├─ total_amount
  └─ rental_items (JSON)
     │
     ├────────────────────────────────────────────────┐
     │ (order_id)                                     │ (order_id)
     ▼                                                ▼
order_items (*)                              product_bookings (*)
  ├─ id (PK)                                   ├─ id (PK)
  ├─ order_id (FK)                            ├─ product_id (FK) → /data/products
  ├─ product_id                               ├─ booked_date
  ├─ base_price                               └─ order_id (FK)
  └─ item_total


transfers (*)
  ├─ id (PK)
  ├─ sender_name
  ├─ amount
  ├─ order_title "CARGO-XXXX"
  ├─ order_id (FK) ───────┐
  ├─ status               │ can find order
  └─ received_at          │
                          ▼
                      (relates to orders table)

shopping_carts (*)
  ├─ id (PK) session-based
  ├─ user_id (FK, nullable)
  ├─ items (JSON)
  │  └─ [{ product_id, start_date, end_date, selected_addons }]
  └─ expires_at
```

---

## HTTP Request/Response Patterns (HTMX)

### Pattern 1: Full Page Navigation
```http
GET /checkout HTTP/1.1
Host: cargo.mleczki.pl

HTTP/1.1 200 OK
Content-Type: text/html

<html>
  <head>...</head>
  <body>
    <header>...</header>
    <main>
      <div id="checkout-form">
        <!-- Full checkout page -->
      </div>
    </main>
    <footer>...</footer>
  </body>
</html>
```

### Pattern 2: HTMX Fragment Swap
```html
<!-- Product page -->
<button hx-post="/cart/add" 
        hx-target="#cart-count" 
        hx-swap="outerHTML">
  Dodaj do rezerwacji
</button>

<span id="cart-count">0 zł</span>

<!-- POST response (fragment) -->
<span id="cart-count">150 zł (1)</span>  <!-- Gets swapped in -->
```

### Pattern 3: HTMX Polling
```html
<!-- Payment page -->
<div id="payment-status" 
     hx-get="/payment/CARGO-1234/status"
     hx-trigger="every 3s"
     hx-swap="outerHTML">
  <div class="loader">Oczekujemy...</div>
</div>

<!-- First poll (0-3s, 3-6s, etc) -->
HTTP 200
Content: (empty or same loader fragment)

<!-- When payment received (after 30+ seconds) -->
HTTP 200
Content:
<div id="payment-status">
  <div class="success">✓ Płatność potwierdzona!</div>
</div>
<!-- HTMX removes hx-trigger automatically -->
```

### Pattern 4: Inline Form Submission
```html
<!-- Cart item removal -->
<div class="cart-item" id="item-123">
  <span>Product Name</span>
  <button hx-post="/cart/remove/123"
          hx-target="#item-123"
          hx-swap="outerHTML swap:1s">
    Remove
  </button>
</div>

<!-- POST response -->
<!-- Empty response + HTMX deletes the div -->
```

---

This architecture supports:
- ✅ Event sourcing for audit trails
- ✅ Scalable read models (could add PostgreSQL later)
- ✅ Async processing (email worker in background)
- ✅ Real-time UI updates (HTMX polling)
- ✅ Stateless backend (sessions in DB, not memory)
- ✅ Simple deployment (single binary + SQLite)


