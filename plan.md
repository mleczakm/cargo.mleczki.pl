







Based on the mockup.tsx, here's the plan for new admin panel features:

## Admin Panel Feature Plan

### **1. Admin Authentication** ✅ SOLVED
- Fix logging, add tests for covering that it works - it fails currently with 405 error ✅
- Add middleware to protect admin routes ✅
- Verify admin session before accessing admin panel ✅

### **2. Admin Panel Structure** ✅ SOLVED
Create tabbed interface with 4 main sections:
- **Orders & Payments** (Zamówienia & Wpłaty) ✅
- **Customers/CRM** (Klienci (CRM)) ✅
- **Fleet Management** (Zarządzanie Flotą) ✅
- **Global Store Closures** (Globalne Zamknięcia Sklepu) ✅

### **3. Orders & Payments Tab** ✅ SOLVED
- List recent orders with status badges (opłacone, oczekuje na płatność) ✅
- Click customer name to navigate to CRM details ✅
- Actions: "Potwierdź" and "Oznacz opłacone" ✅
- BLIK transfer parsing section showing unmatched/matched transfers from email ✅

### **4. Customers/CRM Tab** ✅ SOLVED
- Grid of customer cards with name, email, phone ✅
- Click customer to view details ✅
- **Manual reservation form** for selected customer ✅:
    - Product selection dropdown ✅
    - Custom pricing (for discounts/special cases) ✅
    - Date range selection ✅
    - Creates order and blocks calendar dates automatically ✅

### **5. Fleet Management Tab** ✅ SOLVED
- Grid of products showing blocked dates count ✅
- Click product to manage availability ✅
- **Per-product date blocking** ✅:
    - Block single dates or ranges (maintenance, personal use) ✅
    - List active blocks with unblock button ✅
    - Updates public calendar immediately ✅

### **6. Global Store Closures Tab** ⚠️ PARTIALLY SOLVED
- Add dates (ranges) when entire store is closed (holidays, vacation) ⚠️ (UI exists, backend handler missing)
- List of closed dates with reopen button ⚠️ (UI exists, backend handler missing)
- Affects all products simultaneously ✅ (logic implemented in calendar)

### **7. Database Changes Required** ✅ SOLVED
- Add `global_blocked_dates` table ✅
- Update product availability logic to check global closures ✅
- Add manual reservation order type ✅
