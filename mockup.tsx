import React, { useState, useEffect, useMemo } from 'react';
import {
  ShoppingCart, Bike, Users, Car, Box, Baby, CloudRain,
  Umbrella, Smile, Calendar, CheckCircle, CreditCard,
  ChevronRight, Menu, X, Facebook, Instagram, Phone, Mail,
  Info, ArrowRight, Loader2, User, Settings, ShieldCheck,
  FileText, CalendarDays, Check, Banknote, ListOrdered, Shield,
  ArrowLeft, MapPin, AlertCircle, CalendarClock, Target, Layers
} from 'lucide-react';

// --- CUSTOM SVG COMPONENT: LONG-JOHN CARGO BIKE LOGO ---
// Minimalist representation of a long-john bike (rider, frame, cargo area in front)
const CargoBikeIcon = ({ className, color = 'currentColor', ...props }) => (
    <svg
        viewBox="0 0 100 100"
        className={className}
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
        stroke={color}
        strokeWidth="6"
        strokeLinecap="round"
        strokeLinejoin="round"
        {...props}
    >
      {/* Frame and Handlebars */}
      <path d="M10 70 H40 V40 H70 L90 20" />
      {/* Cargo Area */}
      <rect x="42" y="42" width="26" height="26" rx="4" />
      {/* Wheels: Larger rear, smaller front */}
      <circle cx="20" cy="70" r="10" />
      <circle cx="80" cy="70" r="6" />
      {/* Rider handlebars grip */}
      <path d="M88 18 H94" />
      {/* Saddle */}
      <path d="M35 40 V30 H40" />
    </svg>
);

// --- MOCK DATA ---
const PRODUCTS = [
  {
    id: 'cargo',
    name: 'Rower Cargo (Longjohn)',
    description: 'Najpopularniejszy model. Zwinny, szybki, z paką z przodu na dzieci, zakupy lub sprzęt. Nisko zawieszony środek ciężkości ułatwia jazdę.',
    basePrice: 100,
    image: 'https://images.unsplash.com/photo-1616972041269-e580e224e75d?auto=format&fit=crop&q=80&w=800',
    icon: <CargoBikeIcon className="w-6 h-6" />, // Custom logo used here
    addons: [
      { id: 'daszek', name: 'Daszek przeciwdeszczowy', price: 15, icon: <Umbrella className="w-4 h-4" /> },
      { id: 'poncho', name: 'Poncho dla kierującego', price: 10, icon: <CloudRain className="w-4 h-4" /> },
      { id: 'poduszki', name: 'Dodatkowe poduszki dla dzieci', price: 5, icon: <Smile className="w-4 h-4" /> }
    ]
  },
  {
    id: 'tandem',
    name: 'Rower Tandem',
    description: 'Radość z jazdy we dwoje! Idealny na romantyczne wycieczki lub wyprawy z przyjacielem po okolicy.',
    basePrice: 80,
    image: 'https://images.unsplash.com/photo-1517406692737-142f132e4823?auto=format&fit=crop&q=80&w=800',
    icon: <Users className="w-6 h-6" />,
    addons: []
  },
  {
    id: 'trailer-kids',
    name: 'Przyczepka dla 2 dzieci',
    description: 'Bezpieczna, wygodna, z pasami bezpieczeństwa. Pasuje do każdego naszego roweru (poza cargo/tandem).',
    basePrice: 45,
    image: 'https://images.unsplash.com/photo-1621245037947-f07a049f57eb?auto=format&fit=crop&q=80&w=800',
    icon: <Baby className="w-6 h-6" />,
    addons: []
  },
  {
    id: 'trailer-pedals',
    name: 'Doczepka z pedałami',
    description: 'Dla starszego dziecka, które chce aktywnie uczestniczyć w pedałowaniu podczas wycieczki.',
    basePrice: 40,
    image: 'https://images.unsplash.com/photo-1471506480208-91b3a4cc78be?auto=format&fit=crop&q=80&w=800',
    icon: <Bike className="w-6 h-6" />,
    addons: []
  },
  {
    id: 'trailer-cargo',
    name: 'Przyczepka towarowa',
    description: 'Przewieź duże zakupy, sprzęt na biwak lub inne gabaryty w wygodny sposób.',
    basePrice: 30,
    image: 'https://images.unsplash.com/photo-1628744448840-55bdb2497bbf?auto=format&fit=crop&q=80&w=800',
    icon: <Box className="w-6 h-6" />,
    addons: []
  },
  {
    id: 'car-rack',
    name: 'Bagażnik samochodowy na 2 rowery',
    description: 'Montowany na hak holowniczy. Łatwy w obsłudze i bezpieczny. Do transportu rowerów.',
    basePrice: 35,
    image: 'https://images.unsplash.com/photo-1542362567-b07e54358753?auto=format&fit=crop&q=80&w=800',
    icon: <Car className="w-6 h-6" />,
    addons: []
  }
];

// MOCK BRAND ASSETS (explicit long-john bike usage)
const BRAND_ASSETS = [
  { id: 'logo-full-light', name: 'Logo Główne (Jasne)', desc: 'Pełny logotyp z hasłem, na ciemne tła.', type: 'Logotyp', scale: '100%', variant: 'full' },
  { id: 'logo-full-dark', name: 'Logo Główne (Ciemne)', desc: 'Pełny logotyp z hasłem, na jasne tła.', type: 'Logotyp', scale: '100%', variant: 'full' },
  { id: 'logo-icon-rounded', name: 'Ikona Aplikacyjna (Zaokrąglona)', desc: 'Do użytku w Social Mediach, iOS Home Screen.', type: 'Ikona Profilowa', scale: '1:1', variant: 'icon' },
  { id: 'logo-icon-square', name: 'Ikona Profilowa (Kwadrat)', desc: 'Profilowe Facebook, Instagram.', type: 'Ikona Profilowa', scale: '1:1', variant: 'icon' },
  { id: 'apple-touch-icon', name: 'Apple Touch Icon (iPad)', desc: 'Ikona dla urządzeń Apple (iPad, iPhone).', type: 'Systemowa', size: '180x180 px', format: 'PNG' },
  { id: 'android-icon-512', name: 'Android Chrome Icon', desc: 'Duża ikona aplikacyjna Android.', type: 'Systemowa', size: '512x512 px', format: 'PNG' },
  { id: 'favicon-32', name: 'Favicon Standard', desc: 'Ikona paska adresu przeglądarki.', type: 'Systemowa', size: '32x32 px', format: 'ICO/PNG' },
  { id: 'favicon-16', name: 'Favicon Mały', desc: 'Mała ikona paska adresu.', type: 'Systemowa', size: '16x16 px', format: 'PNG' }
];

const MOCK_TRANSFERS = [
  { id: 't1', date: '2026-06-04 10:15', sender: 'Jan Kowalski', title: 'CARGO-1234', amount: 150, status: 'nieprzypisany' },
  { id: 't2', date: '2026-06-03 14:20', sender: 'Anna Nowak', title: 'CARGO-9981', amount: 80, status: 'dopasowany' }
];

const MOCK_ORDERS = [
  { id: 'CARGO-1234', date: '2026-06-04', user: 'Jan Kowalski', amount: 150, status: 'oczekuje na płatność', payment: 'blik', items: 'Rower Cargo (Longjohn)' },
  { id: 'CARGO-9981', date: '2026-06-03', user: 'Anna Nowak', amount: 80, status: 'opłacone', payment: 'blik', items: 'Rower Tandem' },
  { id: 'CARGO-5555', date: '2026-06-02', user: 'Piotr Wiśniewski', amount: 100, status: 'zaakceptowane', payment: 'gotówka', items: 'Rower Cargo + Daszek' }
];

export default function App() {
  const [currentView, setCurrentView] = useState('home'); // home, product, checkout, payment, success, user, admin, branding
  const [selectedProduct, setSelectedProduct] = useState(null);
  const [cart, setCart] = useState([]);
  const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false);
  const [isLoginModalOpen, setIsLoginModalOpen] = useState(false);
  const [user, setUser] = useState(null); // Simple mock user

  // User & Checkout Details
  const [userDetails, setUserDetails] = useState({
    name: '', email: '', phone: '', address: '', isAdult: false, acceptTos: false, password: ''
  });
  const [paymentMethod, setPaymentMethod] = useState('blik'); // blik, cash

  // Admin State
  const [adminOrders, setAdminOrders] = useState(MOCK_ORDERS);

  // Calculate global totals from cart
  const { cartTotal, addonsTotal, finalTotal } = useMemo(() => {
    let base = 0;
    let addonsPrice = 0;
    cart.forEach(item => {
      base += (item.basePrice * item.rentalDays);
      if (item.selectedAddons) {
        item.selectedAddons.forEach(addon => { addonsPrice += (addon.price * item.rentalDays); });
      }
    });
    return { cartTotal: base, addonsTotal: addonsPrice, finalTotal: base + addonsPrice };
  }, [cart]);

  const navigateTo = (view, product = null) => {
    setCurrentView(view);
    if (product) setSelectedProduct(product);
    setIsMobileMenuOpen(false);
    window.scrollTo(0, 0);
  };

  const removeFromCart = (cartId) => { setCart(cart.filter(item => item.cartId !== cartId)); };
  const markOrderPaid = (id) => { setAdminOrders(adminOrders.map(o => o.id === id ? { ...o, status: 'opłacone' } : o)); };

  const handleLogin = (e) => {
    e.preventDefault();
    setUser({ name: 'Jan Kowalski', email: 'jan@example.com', phone: '123 456 789', address: 'ul. Przykładowa 12/3, 05-250 Radzymin' });
    setUserDetails({...userDetails, name: 'Jan Kowalski', email: 'jan@example.com', phone: '123 456 789', address: 'ul. Przykładowa 12/3, 05-250 Radzymin'});
    setIsLoginModalOpen(false);
  };

  const handleCheckoutSubmit = () => {
    if (!userDetails.name || !userDetails.email || !userDetails.phone || !userDetails.address || (!user && !userDetails.password)) {
      alert('Wypełnij wszystkie dane kontaktowe, adresowe i hasło.'); return;
    }
    if (!userDetails.isAdult || !userDetails.acceptTos) {
      alert('Musisz potwierdzić pełnoletność oraz zaakceptować regulamin, aby wypożyczyć sprzęt.'); return;
    }
    if (!user) setUser({...userDetails}); // Simple register at checkout
    navigateTo('payment');
  };

  // --- SUBCOMPONENTS ---

  const Header = () => (
      <header className="sticky top-0 z-50 bg-white border-b border-gray-100 shadow-sm">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
          <div className="flex justify-between items-center h-20">
            <div className="flex-shrink-0 flex items-center cursor-pointer" onClick={() => navigateTo('home')}>
              {/* Custom Long-John Cargo Bike Logo */}
              <CargoBikeIcon className="h-9 w-9 text-emerald-600" />
              <span className="ml-3 text-2xl font-black tracking-tighter text-gray-900">
              cargo.<span className="text-emerald-600">mleczki</span>.pl
            </span>
            </div>

            <nav className="hidden md:flex space-x-6 items-center">
              <button onClick={() => navigateTo('home')} className="text-gray-600 hover:text-emerald-600 font-medium transition-colors">Oferta</button>
              {user ? (
                  <button onClick={() => navigateTo('user')} className="text-gray-600 hover:text-emerald-600 font-medium transition-colors flex items-center"><User className="w-4 h-4 mr-1"/> Panel Klienta</button>
              ) : (
                  <button onClick={() => setIsLoginModalOpen(true)} className="text-gray-600 hover:text-emerald-600 font-medium transition-colors flex items-center"><User className="w-4 h-4 mr-1"/> Zaloguj się</button>
              )}
              <button onClick={() => navigateTo('admin')} className="text-gray-600 hover:text-red-600 font-medium transition-colors flex items-center"><Settings className="w-4 h-4 mr-1"/> Admin</button>

              <div className="relative cursor-pointer group ml-4" onClick={() => navigateTo('checkout')}>
                <div className="flex items-center space-x-2 bg-emerald-50 text-emerald-700 px-4 py-2 rounded-full font-medium hover:bg-emerald-100 transition-colors">
                  <ShoppingCart className="w-5 h-5" />
                  <span>{cart.length > 0 ? `${finalTotal} zł` : 'Koszyk (0)'}</span>
                </div>
                {cart.length > 0 && (
                    <span className="absolute -top-1 -right-1 bg-red-500 text-white text-xs font-bold rounded-full w-5 h-5 flex items-center justify-center animate-bounce">
                  {cart.length}
                </span>
                )}
              </div>
            </nav>

            <div className="md:hidden flex items-center space-x-4">
              <div className="relative cursor-pointer" onClick={() => navigateTo('checkout')}>
                <ShoppingCart className="w-6 h-6 text-gray-700" />
                {cart.length > 0 && <span className="absolute -top-2 -right-2 bg-red-500 text-white text-xs font-bold rounded-full w-5 h-5 flex items-center justify-center">{cart.length}</span>}
              </div>
              <button onClick={() => setIsMobileMenuOpen(!isMobileMenuOpen)} className="text-gray-700 hover:text-emerald-600">
                {isMobileMenuOpen ? <X className="w-7 h-7" /> : <Menu className="w-7 h-7" />}
              </button>
            </div>
          </div>
        </div>
        {isMobileMenuOpen && (
            <div className="md:hidden bg-white border-b border-gray-100 absolute w-full shadow-lg">
              <div className="px-4 pt-2 pb-6 space-y-2">
                <button onClick={() => navigateTo('home')} className="block w-full text-left px-3 py-3 text-base font-medium text-gray-700 hover:bg-emerald-50 rounded-lg">Oferta</button>
                {user ? (
                    <button onClick={() => navigateTo('user')} className="block w-full text-left px-3 py-3 text-base font-medium text-gray-700 hover:bg-emerald-50 rounded-lg">Panel Klienta</button>
                ) : (
                    <button onClick={() => setIsLoginModalOpen(true)} className="block w-full text-left px-3 py-3 text-base font-medium text-gray-700 hover:bg-emerald-50 rounded-lg">Zaloguj się</button>
                )}
                <button onClick={() => navigateTo('admin')} className="block w-full text-left px-3 py-3 text-base font-medium text-red-600 hover:bg-red-50 rounded-lg">Panel Administratora</button>
                <button onClick={() => navigateTo('checkout')} className="block w-full text-left px-3 py-3 text-base font-medium text-emerald-600 bg-emerald-50 rounded-lg flex justify-between items-center">
                  <span>Koszyk ({cart.length})</span>
                  <span className="font-bold">{finalTotal} zł</span>
                </button>
              </div>
            </div>
        )}
      </header>
  );

  const ProductCard = ({ product }) => (
      <div className="bg-white rounded-2xl shadow-sm border border-gray-100 overflow-hidden hover:shadow-xl transition-shadow duration-300 flex flex-col cursor-pointer" onClick={() => navigateTo('product', product)}>
        <div className="h-56 overflow-hidden relative group">
          <img src={product.image} alt={product.name} className="w-full h-full object-cover group-hover:scale-105 transition-transform duration-500" />
          <div className="absolute top-4 left-4 bg-white/90 backdrop-blur px-3 py-1.5 rounded-full flex items-center space-x-2 shadow-sm border border-gray-100">
            {product.icon}
            <span className="font-bold text-gray-900">{product.basePrice} zł / dobę</span>
          </div>
        </div>
        <div className="p-6 flex-grow flex flex-col">
          <h3 className="text-xl font-bold text-gray-900 mb-2">{product.name}</h3>
          <p className="text-gray-600 text-sm mb-6 flex-grow line-clamp-2">{product.description}</p>
          <button className="w-full bg-gray-100 hover:bg-emerald-600 hover:text-white text-gray-900 font-medium py-3 px-4 rounded-xl transition-colors duration-300 flex items-center justify-center space-x-2">
            <CalendarDays className="w-5 h-5" />
            <span>Sprawdź dostępność i rezerwuj</span>
          </button>
        </div>
      </div>
  );

  const ProductDetailView = () => {
    const [itemStartDate, setItemStartDate] = useState('');
    const [itemEndDate, setItemEndDate] = useState('');
    const [selectedAddons, setSelectedAddons] = useState([]);

    const itemRentalDays = useMemo(() => {
      if (!itemStartDate || !itemEndDate) return 1;
      const start = new Date(itemStartDate);
      const end = new Date(itemEndDate);
      if (end < start) return 1; // Error case
      const diffDays = Math.ceil(Math.abs(end - start) / (1000 * 60 * 60 * 24));
      return diffDays >= 0 ? diffDays + 1 : 1;
    }, [itemStartDate, itemEndDate]);

    const itemTotal = useMemo(() => {
      let addonsPrice = selectedAddons.reduce((acc, curr) => acc + curr.price, 0);
      return (selectedProduct.basePrice + addonsPrice) * itemRentalDays;
    }, [selectedProduct, selectedAddons, itemRentalDays]);

    const handleAddToCart = () => {
      if (!itemStartDate || !itemEndDate) { alert("Wybierz daty wynajmu w kalendarzu dostępności."); return; }
      setCart([...cart, { ...selectedProduct, cartId: Date.now(), selectedAddons, startDate: itemStartDate, endDate: itemEndDate, rentalDays: itemRentalDays }]);
      navigateTo('checkout');
    };

    const toggleAddon = (addon) => {
      if (selectedAddons.find(a => a.id === addon.id)) { setSelectedAddons(selectedAddons.filter(a => a.id !== addon.id));
      } else { setSelectedAddons([...selectedAddons, addon]); }
    };

    return (
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-12">
          <button onClick={() => navigateTo('home')} className="flex items-center text-gray-500 hover:text-emerald-600 mb-8 transition-colors"><ArrowLeft className="w-5 h-5 mr-2" /> Wróć do oferty</button>
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-12">
            <div className="rounded-3xl overflow-hidden shadow-lg h-96 lg:h-[600px] border border-gray-100">
              <img src={selectedProduct.image} alt={selectedProduct.name} className="w-full h-full object-cover" />
            </div>
            <div className="flex flex-col">
              <div className="flex items-center space-x-4 mb-6">
                <div className="p-4 bg-emerald-100 text-emerald-600 rounded-2xl border border-emerald-200">{selectedProduct.icon}</div>
                <h1 className="text-3xl lg:text-4xl font-black text-gray-900 leading-tight">{selectedProduct.name}</h1>
              </div>
              <p className="text-lg text-gray-600 mb-8 leading-relaxed">{selectedProduct.description}</p>
              <div className="bg-gray-50 border border-gray-200 rounded-3xl p-8 mb-8 shadow-inner">
                <h3 className="text-xl font-bold text-gray-900 mb-6 flex items-center"><CalendarClock className="w-6 h-6 mr-2 text-emerald-600" /> Kalendarz Dostępności</h3>
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-6">
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-1.5">Od dnia (Odbiór)</label>
                    <input type="date" value={itemStartDate} onChange={(e) => setItemStartDate(e.target.value)} min={new Date().toISOString().split('T')[0]} className="w-full p-3.5 border border-gray-300 rounded-xl focus:ring-2 focus:ring-emerald-500" />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-1.5">Do dnia (Zwrot)</label>
                    <input type="date" value={itemEndDate} onChange={(e) => setItemEndDate(e.target.value)} min={itemStartDate || new Date().toISOString().split('T')[0]} className="w-full p-3.5 border border-gray-300 rounded-xl focus:ring-2 focus:ring-emerald-500" />
                  </div>
                </div>
                {itemStartDate && itemEndDate && (
                    <div className="mt-6 p-4 bg-emerald-100 text-emerald-800 rounded-xl flex items-center text-sm font-semibold border border-emerald-200">
                      <CheckCircle className="w-5 h-5 mr-2.5" /> Sprzęt jest dostępny w tym terminie! Wynajem na {itemRentalDays} dni.
                    </div>
                )}
              </div>
              {selectedProduct.addons && selectedProduct.addons.length > 0 && (
                  <div className="mb-8">
                    <h3 className="text-lg font-bold text-gray-900 mb-4">Opcje dodatkowe</h3>
                    <div className="space-y-3">
                      {selectedProduct.addons.map(addon => {
                        const isSelected = selectedAddons.find(a => a.id === addon.id);
                        return (
                            <label key={addon.id} className={`flex items-center justify-between p-5 rounded-2xl cursor-pointer transition-colors border-2 ${isSelected ? 'bg-emerald-50 border-emerald-400' : 'bg-white border-gray-200 hover:border-emerald-300'}`}>
                              <div className="flex items-center space-x-4">
                                <input type="checkbox" checked={!!isSelected} onChange={() => toggleAddon(addon)} className="w-5 h-5 text-emerald-600 rounded border-gray-300 focus:ring-emerald-500" />
                                <div className="flex items-center space-x-2.5">
                                  <div className="text-gray-500">{addon.icon}</div>
                                  <span className="font-semibold text-gray-900">{addon.name}</span>
                                </div>
                              </div>
                              <span className="font-bold text-emerald-700">+{addon.price} zł / dobę</span>
                            </label>
                        )
                      })}
                    </div>
                  </div>
              )}
              <div className="mt-auto bg-gray-900 rounded-2xl p-7 text-white flex flex-col sm:flex-row items-center justify-between shadow-lg">
                <div className="mb-4 sm:mb-0 text-center sm:text-left">
                  <p className="text-gray-400 text-sm">Razem za wybrany okres:</p>
                  <p className="text-4xl font-black text-emerald-400">{itemTotal} zł</p>
                </div>
                <button onClick={handleAddToCart} className="w-full sm:w-auto bg-emerald-500 hover:bg-emerald-400 text-white font-bold py-4 px-9 rounded-xl transition-colors flex items-center justify-center space-x-2.5 shadow-md">
                  <ShoppingCart className="w-6 h-6" />
                  <span>Dodaj do rezerwacji</span>
                </button>
              </div>
              <p className="text-xs text-gray-500 text-center mt-4 flex items-center justify-center"><Shield className="w-4 h-4 mr-1.5"/> Płatność szybkim i bezprowizyjnym przelewem BLIK lub gotówką.</p>
            </div>
          </div>
        </div>
    );
  };

  const CheckoutView = () => {
    if (cart.length === 0) {
      return (
          <div className="max-w-3xl mx-auto px-4 py-20 text-center">
            <div className="bg-gray-50 rounded-full w-24 h-24 flex items-center justify-center mx-auto mb-6"><ShoppingCart className="w-12 h-12 text-gray-400" /></div>
            <h2 className="text-2xl font-bold text-gray-900 mb-4">Twój koszyk jest pusty</h2>
            <button onClick={() => navigateTo('home')} className="bg-emerald-600 hover:bg-emerald-700 text-white px-8 py-3 rounded-xl font-semibold transition-colors">Wróć do oferty</button>
          </div>
      );
    }
    return (
        <div className="max-w-7xl mx-auto px-4 py-12 lg:flex lg:space-x-12">
          <div className="lg:w-2/3 space-y-8">
            <h2 className="text-3xl font-black text-gray-900">Dane do umowy i zamówienie</h2>
            <div className="space-y-4">
              <h3 className="text-xl font-bold text-gray-900">Sprzęt w koszyku</h3>
              {cart.map((item) => (
                  <div key={item.cartId} className="bg-white rounded-2xl shadow-sm border border-gray-100 p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4">
                    <div className="flex items-center space-x-4">
                      <div className="w-20 h-20 bg-gray-50 rounded-xl flex items-center justify-center text-gray-400 border border-gray-100">{item.icon}</div>
                      <div>
                        <h4 className="font-bold text-gray-900">{item.name}</h4>
                        <p className="text-sm text-emerald-600 font-medium mt-1">Od {item.startDate} do {item.endDate} ({item.rentalDays} dni)</p>
                        {item.selectedAddons && item.selectedAddons.length > 0 && <div className="mt-2 space-y-1">{item.selectedAddons.map(a => <p key={a.id} className="text-xs text-gray-500">• {a.name}</p>)}</div>}
                      </div>
                    </div>
                    <div className="text-right flex flex-col items-end">
                      <p className="font-bold text-xl text-gray-900">{(item.basePrice + (item.selectedAddons?.reduce((acc, a) => acc + a.price, 0) || 0)) * item.rentalDays} zł</p>
                      <button onClick={() => removeFromCart(item.cartId)} className="text-sm text-red-500 mt-2 hover:underline">Usuń</button>
                    </div>
                  </div>
              ))}
            </div>
            <div className="bg-white rounded-2xl shadow-sm border border-gray-100 p-8">
              <h3 className="text-2xl font-bold text-gray-900 mb-6 flex items-center"><FileText className="w-6 h-6 mr-2 text-emerald-600" /> Dane Najemcy (do umowy)</h3>
              {!user && <p className="text-sm text-gray-500 mb-6 bg-emerald-50 border border-emerald-100 p-3 rounded-lg flex"><User className="w-4 h-4 mr-2"/> Konto zostanie automatycznie utworzone po złożeniu zamówienia.</p>}
              <div className="grid grid-cols-1 md:grid-cols-2 gap-6 mb-8">
                <div className="md:col-span-2">
                  <label className="block text-sm font-medium text-gray-700 mb-1">Imię i nazwisko</label>
                  <input type="text" value={userDetails.name} onChange={(e) => setUserDetails({...userDetails, name: e.target.value})} className="w-full p-3.5 border border-gray-300 rounded-xl focus:ring-2 focus:ring-emerald-500" placeholder="Jan Kowalski" />
                </div>
                <div className="md:col-span-2">
                  <label className="block text-sm font-medium text-gray-700 mb-1">Adres zamieszkania</label>
                  <input type="text" value={userDetails.address} onChange={(e) => setUserDetails({...userDetails, address: e.target.value})} className="w-full p-3.5 border border-gray-300 rounded-xl focus:ring-2 focus:ring-emerald-500" placeholder="ul. Przykładowa 12/3, 05-250 Radzymin" />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">Adres e-mail</label>
                  <input type="email" value={userDetails.email} onChange={(e) => setUserDetails({...userDetails, email: e.target.value})} className="w-full p-3.5 border border-gray-300 rounded-xl focus:ring-2 focus:ring-emerald-500" placeholder="jan@example.com" />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">Telefon</label>
                  <input type="tel" value={userDetails.phone} onChange={(e) => setUserDetails({...userDetails, phone: e.target.value})} className="w-full p-3.5 border border-gray-300 rounded-xl focus:ring-2 focus:ring-emerald-500" placeholder="123 456 789" />
                </div>
                {!user && (
                    <div className="md:col-span-2">
                      <label className="block text-sm font-medium text-gray-700 mb-1">Hasło do nowego konta (wymagane)*</label>
                      <input type="password" value={userDetails.password} onChange={(e) => setUserDetails({...userDetails, password: e.target.value})} className="w-full p-3.5 border border-gray-300 rounded-xl focus:ring-2 focus:ring-emerald-500" placeholder="••••••••" />
                    </div>
                )}
              </div>
              <div className="space-y-4 border-t border-gray-100 pt-8">
                <label className="flex items-start space-x-3 cursor-pointer"><input type="checkbox" checked={userDetails.isAdult} onChange={(e) => setUserDetails({...userDetails, isAdult: e.target.checked})} className="mt-1 w-5 h-5 text-emerald-600 rounded border-gray-300 focus:ring-emerald-500" /><span className="text-sm text-gray-700">Potwierdzam, że jestem osobą pełnoletnią i posiadam dokument tożsamości do wglądu przy odbiorze sprzętu.*</span></label>
                <label className="flex items-start space-x-3 cursor-pointer"><input type="checkbox" checked={userDetails.acceptTos} onChange={(e) => setUserDetails({...userDetails, acceptTos: e.target.checked})} className="mt-1 w-5 h-5 text-emerald-600 rounded border-gray-300 focus:ring-emerald-500" /><span className="text-sm text-gray-700">Akceptuję <a href="#" className="text-emerald-600 underline">Regulamin i Umowę Najmu</a>. Wyrażam zgodę na przetwarzanie danych osobowych (RODO).*</span></label>
              </div>
            </div>
            <div className="bg-white rounded-2xl shadow-sm border border-gray-100 p-8">
              <h3 className="text-xl font-bold text-gray-900 mb-6">Metoda płatności</h3>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-6">
                <label className={`flex flex-col items-center p-6 border-2 rounded-2xl cursor-pointer transition-all ${paymentMethod === 'blik' ? 'border-emerald-500 bg-emerald-50' : 'border-gray-200 hover:border-emerald-300'}`}>
                  <input type="radio" name="payment" className="hidden" checked={paymentMethod === 'blik'} onChange={() => setPaymentMethod('blik')} />
                  <CreditCard className={`w-10 h-10 mb-3 ${paymentMethod === 'blik' ? 'text-emerald-600' : 'text-gray-400'}`} />
                  <span className="font-bold text-gray-900">Szybki Przelew BLIK</span>
                  <span className="text-xs text-center mt-1.5 text-gray-500 leading-relaxed">Bezpieczny przelew na telefon (autorski system, brak prowizji).</span>
                </label>
                <label className={`flex flex-col items-center p-6 border-2 rounded-2xl cursor-pointer transition-all ${paymentMethod === 'cash' ? 'border-emerald-500 bg-emerald-50' : 'border-gray-200 hover:border-emerald-300'}`}>
                  <input type="radio" name="payment" className="hidden" checked={paymentMethod === 'cash'} onChange={() => setPaymentMethod('cash')} />
                  <Banknote className={`w-10 h-10 mb-3 ${paymentMethod === 'cash' ? 'text-emerald-600' : 'text-gray-400'}`} />
                  <span className="font-bold text-gray-900">Gotówka przy odbiorze</span>
                  <span className="text-xs text-center mt-1.5 text-gray-500">Płatność na miejscu (Radzymin), odliczona kwota.</span>
                </label>
              </div>
            </div>
          </div>
          <div className="lg:w-1/3 mt-12 lg:mt-0">
            <div className="bg-gray-900 rounded-3xl p-8 text-white sticky top-24 shadow-2xl">
              <h3 className="text-2xl font-bold mb-6">Podsumowanie</h3>
              <div className="space-y-4 mb-6 text-gray-300">
                <div className="flex justify-between"><span>Koszt sprzętu</span><span className="font-medium text-white">{cartTotal} zł</span></div>
                {addonsTotal > 0 && <div className="flex justify-between"><span>Opcje dodatkowe</span><span className="font-medium text-white">{addonsTotal} zł</span></div>}
                <div className="border-t border-gray-700 my-4"></div>
                <div className="flex justify-between text-2xl font-black text-white"><span>Razem</span><span className="text-emerald-400">{finalTotal} zł</span></div>
              </div>
              <button onClick={handleCheckoutSubmit} className="w-full bg-emerald-500 hover:bg-emerald-400 text-white font-bold py-4 px-4 rounded-xl transition-colors flex items-center justify-center text-lg shadow-md">
                Potwierdzam rezerwację <ChevronRight className="ml-2 w-6 h-6" />
              </button>
              <p className="text-xs text-gray-500 text-center mt-6 flex items-center justify-center"><CheckCircle className="w-4 h-4 mr-1.5 text-emerald-500"/> Złóż zamówienie, a następnie dokonaj płatności.</p>
            </div>
          </div>
        </div>
    );
  };

  const PaymentView = () => {
    const [isVerifying, setIsVerifying] = useState(false);
    const handleSimulatePayment = () => { if (paymentMethod === 'cash') { navigateTo('success'); return; }
      setIsVerifying(true); setTimeout(() => { setIsVerifying(false); navigateTo('success'); }, 4000); };

    if (paymentMethod === 'cash') {
      return (
          <div className="max-w-2xl mx-auto px-4 py-20 text-center">
            <Banknote className="w-20 h-20 text-emerald-600 mx-auto mb-6" />
            <h2 className="text-4xl font-black mb-4">Płatność gotówką</h2>
            <p className="text-lg text-gray-600 mb-8 leading-relaxed">Wybrałeś płatność gotówką na miejscu. Rezerwacja jest już widoczna w systemie. Przygotuj prosimy odliczoną kwotę ({finalTotal} zł) przy odbiorze sprzętu.</p>
            <button onClick={handleSimulatePayment} className="bg-emerald-600 hover:bg-emerald-700 text-white px-8 py-4 rounded-xl font-bold text-lg shadow-lg">Rozumiem, sfinalizuj rezerwację</button>
          </div>
      );
    }

    return (
        <div className="max-w-3xl mx-auto px-4 py-16">
          <div className="bg-white rounded-3xl shadow-xl border border-gray-100 overflow-hidden text-center">
            <div className="bg-gray-900 text-white p-10 flex flex-col items-center">
              <CreditCard className="w-16 h-16 mb-4 text-emerald-400" />
              <h2 className="text-3xl font-black mb-2 leading-tight">Płatność Szybkim Przelewem BLIK</h2>
              <p className="text-emerald-300 text-sm font-medium">Autorski system bezpośredni, brak prowizji.</p>
            </div>
            <div className="p-10">
              <div className="bg-emerald-50 border-2 border-emerald-100 rounded-3xl p-8 mb-8 text-left shadow-inner">
                <h3 className="font-bold text-gray-900 mb-6 flex items-center"><Info className="w-6 h-6 mr-2.5 text-emerald-600" /> Instrukcja płatności (trwa 30 sekund)</h3>
                <ol className="list-decimal list-inside space-y-4 text-gray-700 font-medium">
                  <li>Zaloguj się do aplikacji banku i wybierz "Przelew BLIK na telefon".</li>
                  <li>Jako odbiorcę wpisz numer: <span className="font-black text-gray-900">500 123 456</span></li>
                  <li>Wpisz kwotę przelewu: <span className="font-black text-emerald-700 text-lg">{finalTotal} zł</span></li>
                  <li>Tytuł (BARDZO WAŻNE):<br/><span className="inline-block bg-white border border-gray-200 px-4 py-2.5 mt-2 rounded-lg font-mono text-emerald-700 font-bold border-emerald-100">CARGO-{Math.floor(Math.random()*10000)}</span></li>
                </ol>
              </div>
              {isVerifying ? (
                  <div className="flex flex-col items-center justify-center py-6">
                    <Loader2 className="w-12 h-12 text-emerald-600 animate-spin mb-4" />
                    <p className="font-bold text-gray-900 text-lg">Oczekujemy na potwierdzenie wpłaty przez serwer...</p>
                    <p className="text-sm text-gray-500 mt-2">Działa to automatycznie, trwa zazwyczaj od kilku do 120 sekund.</p>
                  </div>
              ) : (
                  <button onClick={handleSimulatePayment} className="w-full bg-emerald-600 hover:bg-emerald-700 text-white font-bold py-4.5 px-6 rounded-2xl transition-all shadow-lg text-lg">Wysłałem przelew BLIK</button>
              )}
            </div>
          </div>
        </div>
    );
  };

  const SuccessView = () => (
      <div className="max-w-2xl mx-auto px-4 py-24 text-center">
        <div className="bg-emerald-100 w-24 h-24 rounded-full flex items-center justify-center mx-auto mb-8 border-2 border-emerald-200"><CheckCircle className="w-12 h-12 text-emerald-600" /></div>
        <h2 className="text-4xl font-black text-gray-900 mb-4">Udało się! Do zobaczenia w Radzyminie.</h2>
        <p className="text-xl text-gray-600 mb-10 leading-relaxed">Twoja rezerwacja została potwierdzona. Wszystkie szczegóły oraz Wzór Umowy wysłaliśmy na e-mail: <strong>{userDetails.email}</strong>. Pamiętaj o zabraniu dokumentu tożsamości.</p>
        <button onClick={() => { setCart([]); navigateTo('user'); }} className="bg-gray-900 hover:bg-gray-800 text-white px-9 py-4 rounded-xl font-semibold transition-colors text-lg">Przejdź do Twojego Panelu Klienta</button>
      </div>
  );

  const UserPanelView = () => (
      <div className="max-w-5xl mx-auto px-4 py-12">
        <h2 className="text-3xl font-black text-gray-900 mb-8">Panel Klienta</h2>
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
          <div className="lg:col-span-1"><div className="bg-white rounded-2xl border border-gray-100 p-7 shadow-sm"><div className="w-16 h-16 bg-emerald-100 rounded-full flex items-center justify-center mb-4 text-emerald-600 border border-emerald-200"><User className="w-8 h-8" /></div><h3 className="font-bold text-xl mb-1">{userDetails.name || 'Michał (Demo Client)'}</h3><p className="text-gray-500 text-sm mb-4">{userDetails.email || 'michal@example.com'}</p><div className="border-t border-gray-100 pt-4 text-sm text-gray-600 space-y-2.5"><p><strong>Tel:</strong> {userDetails.phone || '---'}</p><p><strong>Adres:</strong> {userDetails.address || '---'}</p></div><button className="w-full bg-red-50 text-red-700 text-xs mt-6 p-2 rounded hover:bg-red-100 flex items-center justify-center"><AlertCircle className="w-3.5 h-3.5 mr-1.5"/> Zażądaj usunięcia konta (RODO)</button></div></div>
          <div className="lg:col-span-2 space-y-4"><h3 className="font-bold text-xl text-gray-900 mb-4 flex items-center"><ListOrdered className="w-5 h-5 mr-2" /> Twoje rezerwacje</h3><div className="bg-white rounded-2xl border border-emerald-200 p-6 shadow-sm"><div className="flex justify-between items-start mb-4"><div><span className="bg-emerald-100 text-emerald-800 text-xs font-bold px-2.5 py-1 rounded-full">Aktywna</span><h4 className="font-bold text-lg mt-2">CARGO-TEST</h4><p className="text-sm text-gray-500">Najbliższy wynajem</p></div><div className="text-right"><p className="font-bold text-gray-900">{finalTotal > 0 ? finalTotal : 150} zł</p><p className="text-xs text-gray-500">{paymentMethod === 'blik' ? 'Opłacone (BLIK)' : 'Do zapłaty gotówką'}</p></div></div></div><div className="bg-gray-50 rounded-2xl border border-gray-200 p-6 opacity-70"><div className="flex justify-between items-start"><div><span className="bg-gray-200 text-gray-600 text-xs font-bold px-2.5 py-1 rounded-full">Zakończona</span><h4 className="font-bold text-lg mt-2">CARGO-0012</h4><p className="text-sm text-gray-500">Lipiec 2025</p></div></div></div></div>
        </div>
      </div>
  );

  const AdminPanelView = () => (
      <div className="max-w-7xl mx-auto px-4 py-12">
        <div className="flex items-center justify-between mb-8"><h2 className="text-3xl font-black text-gray-900 flex items-center"><Settings className="w-8 h-8 mr-3 text-red-500"/> Panel Administratora</h2>
          {/* New branding management button */}
          <button onClick={() => navigateTo('branding')} className="flex items-center space-x-2.5 bg-gray-900 hover:bg-gray-800 text-white px-5 py-3 rounded-xl font-semibold shadow">
            <Layers className="w-5 h-5 text-emerald-400" />
            <span>Zarządzaj Brandingiem</span>
          </button>
        </div>
        <div className="bg-blue-50 border border-blue-200 text-blue-800 p-4 rounded-xl mb-8 flex text-sm"><Info className="w-5 h-5 mr-2 flex-shrink-0" /><p>Edycja produktów (Markdown) odbywa się w repozytorium (Git). Tutaj zarządzasz płatnościami i umowami.</p></div>
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-8"><div className="bg-white rounded-2xl shadow-sm border border-gray-200 p-7"><h3 className="font-bold text-xl mb-6 border-b pb-2">Ostatnie zamówienia</h3><div className="space-y-4">{adminOrders.map(order => (<div key={order.id} className="border border-gray-100 bg-gray-50 p-4 rounded-xl"><div className="flex justify-between items-center mb-2"><span className="font-bold text-gray-900">{order.id}</span><span className={`text-xs px-2.5 py-1 rounded-full font-bold ${order.status === 'opłacone' ? 'bg-emerald-100 text-emerald-700' : 'bg-yellow-100 text-yellow-700'}`}>{order.status.toUpperCase()}</span></div><p className="text-sm text-gray-600 mb-1"><User className="w-3 h-3 inline mr-1"/> {order.user}</p><p className="text-sm text-gray-600 mb-3"><CargoBikeIcon className="w-3.5 h-3.5 inline mr-1"/> {order.items}</p><div className="flex justify-between items-center mt-2 pt-2 border-t border-gray-200"><span className="font-bold">{order.amount} zł ({order.payment})</span>{order.status !== 'opłacone' && (<button onClick={() => markOrderPaid(order.id)} className="text-xs bg-emerald-600 text-white px-3 py-1.5 rounded-lg hover:bg-emerald-700">Oznacz jako opłacone</button>)}</div></div>))}</div></div><div className="bg-white rounded-2xl shadow-sm border border-gray-200 p-7"><h3 className="font-bold text-xl mb-6 border-b pb-2 flex items-center justify-between"><span>Parsowanie z Emaila (BLIK)</span><span className="text-xs font-normal bg-gray-200 px-2 py-1 rounded-full text-gray-600">Ostatnia sync: 1 min temu</span></h3><div className="space-y-4">{MOCK_TRANSFERS.map(t => (<div key={t.id} className="border-l-4 border-emerald-500 bg-gray-50 p-4 rounded-r-xl"><div className="flex justify-between"><span className="text-xs text-gray-500">{t.date}</span><span className={`text-xs font-bold ${t.status === 'matched' ? 'text-emerald-600' : 'text-red-500'}`}>{t.status === 'matched' ? 'DOPASOWANY' : 'BRAK DOPASOWANIA'}</span></div><p className="font-bold text-gray-900 mt-1">{t.amount} PLN</p><p className="text-sm text-gray-600">Od: {t.sender}</p><p className="text-sm font-mono text-blue-600 bg-blue-50 inline-block px-2 py-0.5 mt-1 rounded">Tytuł: {t.title}</p></div>))}</div></div></div>
      </div>
  );

  // --- NEW BRANDING ASSET VIEW ---
  const BrandingView = () => (
      <div className="max-w-7xl mx-auto px-4 py-12">
        <div className="flex items-center justify-between mb-8">
          <h2 className="text-3xl font-black text-gray-900 flex items-center">
            <Layers className="w-8 h-8 mr-3 text-emerald-600"/> Biblioteka Assetów Brandingu
          </h2>
          <button onClick={() => navigateTo('admin')} className="flex items-center space-x-2 text-gray-500 hover:text-emerald-600 transition-colors">
            <ArrowLeft className="w-5 h-5" />
            <span>Wróć do Admin Panelu</span>
          </button>
        </div>

        <div className="bg-emerald-50 border border-emerald-100 text-emerald-800 p-5 rounded-2xl mb-10 flex text-sm shadow-inner">
          <Target className="w-5 h-5 mr-3 flex-shrink-0 mt-0.5" />
          <p className="leading-relaxed"><strong>Kluczowa zmiana w logo:</strong> Dotychczasowa, standardowa ikona roweru została zastąpiona dedykowanym customowym symbolem **roweru cargo typu long-john** (charakterystyczna niskoprofilowa platforma transportowa z przodu). Ten motyw jest konsekwentnie stosowany we wszystkich poniższych assetach dla spójności i unikalności brandingu `cargo.mleczki.pl`.</p>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-8 mb-12">
          {/* Render Logos & Profiles (variants) */}
          {BRAND_ASSETS.filter(a => a.type !== 'Systemowa').map(a => (
              <div key={a.id} className="bg-white rounded-2xl shadow border border-gray-100 p-6 flex flex-col">
                <div className={`flex items-center justify-center p-6 rounded-xl ${a.id.includes('light') ? 'bg-gray-900' : 'bg-gray-100'} border border-gray-200 aspect-video mb-5`}>
                  {a.variant === 'full' ? (
                      <div className={`flex items-center ${a.id.includes('light') ? 'text-white' : 'text-gray-900'}`}>
                        <CargoBikeIcon className={`h-10 w-10 ${a.id.includes('light') ? 'text-emerald-400' : 'text-emerald-600'}`} />
                        <span className="ml-3 text-xl font-black tracking-tighter">cargo.<span className={`${a.id.includes('light') ? 'text-emerald-400' : 'text-emerald-600'}`}>mleczki</span>.pl</span>
                      </div>
                  ) : (
                      <div className={`flex items-center justify-center ${a.id.includes('rounded') ? 'rounded-2xl' : ''} ${a.id.includes('icon') ? 'p-3 bg-emerald-600 text-white' : ''} h-20 w-20 border border-emerald-500 shadow`}>
                        <CargoBikeIcon className="h-12 w-12" color={a.id.includes('icon') ? 'white' : 'currentColor'}/>
                      </div>
                  )}
                </div>
                <h3 className="font-bold text-lg text-gray-900">{a.name}</h3>
                <p className="text-sm text-gray-500 mb-2">{a.desc}</p>
                <div className="mt-auto pt-4 border-t border-gray-100 text-xs text-gray-400 flex items-center justify-between">
                  <span>{a.type}</span>
                  <span className="font-mono bg-gray-100 px-1.5 py-0.5 rounded">{a.scale || 'N/A'}</span>
                </div>
              </div>
          ))}
        </div>

        {/* Render System Icons Grid */}
        <h3 className="font-bold text-xl text-gray-900 mb-6">Specyficzne Assety Systemowe (Favicon, iOS, Android, macOS)</h3>
        <div className="grid grid-cols-2 md:grid-cols-4 gap-6">
          {BRAND_ASSETS.filter(a => a.type === 'Systemowa').map(a => (
              <div key={a.id} className="bg-white rounded-xl shadow-sm border border-gray-100 p-5 flex flex-col items-center text-center">
                <div className="flex items-center justify-center p-3 rounded-lg bg-gray-50 border border-gray-100 aspect-square h-20 w-20 mb-4 overflow-hidden shadow-inner">
                  {a.id.includes('favicon') ? (
                      <CargoBikeIcon className={`${a.size.startsWith('16') ? 'h-4 w-4' : 'h-8 w-8'} text-emerald-600`} strokeWidth={a.size.startsWith('16') ? 4 : 6}/>
                  ) : (
                      <div className={`flex items-center justify-center p-2 rounded ${a.id.includes('apple') ? 'bg-white rounded-lg' : 'bg-emerald-600'} border ${a.id.includes('apple') ? 'border-gray-100 shadow' : 'border-emerald-500'}`}>
                        <CargoBikeIcon className={`${a.size.includes('512') ? 'h-10 w-10' : 'h-8 w-8'}`} color={a.id.includes('apple') ? 'black' : 'white'} />
                      </div>
                  )}
                </div>
                <h4 className="font-semibold text-sm text-gray-900 leading-snug">{a.name}</h4>
                <p className="text-xs text-gray-500 mb-2 line-clamp-1">{a.desc}</p>
                <div className="mt-auto w-full pt-3 border-t border-gray-100 text-xs text-gray-400 flex items-center justify-between font-mono">
                  <span className="font-bold text-gray-700">{a.size}</span>
                  <span className="bg-gray-100 px-1 py-0.5 rounded">{a.format}</span>
                </div>
              </div>
          ))}
        </div>
      </div>
  );

  return (
      <div className="min-h-screen bg-white font-sans text-gray-900 flex flex-col">
        <Header />
        <main className="flex-grow">
          {currentView === 'home' && (
              <><div className="bg-emerald-900 text-white py-20 text-center flex flex-col items-center"><CargoBikeIcon className="h-16 w-16 mb-5 text-emerald-400"/><h1 className="text-4xl md:text-5xl lg:text-6xl font-black tracking-tight mb-5 leading-tight">Wynajmij sprzęt na rodzinne wyprawy.</h1><p className="text-emerald-200 text-lg max-w-2xl">Radzymin i okolice. Ceny już od 30 zł / dobę. Bezprowizyjna płatność BLIK.</p></div><section className="py-16 bg-gray-50"><div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-8">{PRODUCTS.map(product => <ProductCard key={product.id} product={product} />)}</div></section></>
          )}
          {currentView === 'product' && selectedProduct && <ProductDetailView />}
          {currentView === 'checkout' && <CheckoutView />}
          {currentView === 'payment' && <PaymentView />}
          {currentView === 'success' && <SuccessView />}
          {currentView === 'user' && <UserPanelView />}
          {currentView === 'admin' && <AdminPanelView />}
          {/* NEW VIEW CONNECTION */}
          {currentView === 'branding' && <BrandingView />}
        </main>
        <footer className="bg-gray-900 text-gray-400 py-16 border-t border-gray-800 text-sm">
          <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 grid grid-cols-1 md:grid-cols-3 gap-10">
            <div><div className="flex items-center text-white mb-5"><CargoBikeIcon className="h-9 w-9 text-emerald-500" /><span className="ml-3 text-xl font-black tracking-tighter">cargo.<span className="text-emerald-500">mleczki</span>.pl</span></div><p className="leading-relaxed">Prywatna wypożyczalnia sprzętu rowerowego w okolicach Radzymina.</p></div>
            <div><h4 className="text-white font-bold mb-5">Informacje Prawne</h4><ul className="space-y-3"><li><a href="#" className="hover:text-emerald-400 transition-colors">Regulamin wypożyczalni</a></li><li><a href="#" className="hover:text-emerald-400 transition-colors">Wzór Umowy Najmu</a></li><li><a href="#" className="hover:text-emerald-400 transition-colors">Polityka Prywatności (RODO)</a></li></ul></div>
            <div><h4 className="text-white font-bold mb-5 flex items-center"><Layers className="w-5 h-5 mr-2 text-emerald-500"/> Branding & Cookies</h4><p className="flex items-start text-xs leading-relaxed"><ShieldCheck className="w-5 h-5 mr-2.5 text-emerald-500 flex-shrink-0 mt-0.5" />Szanujemy prywatność. Logo `cargo.mleczki.pl` przedstawia rower cargo typu **long-john**. Wykorzystujemy wyłącznie niezbędne sesyjne ciasteczka (logowanie/koszyk). Brak skryptów śledzących.</p></div>
          </div>
        </footer>
      </div>
  );
}