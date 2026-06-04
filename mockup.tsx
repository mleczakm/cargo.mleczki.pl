import React, { useState, useEffect, useMemo } from 'react';
import { 
  ShoppingCart, Bike, Users, Car, Box, Baby, CloudRain, 
  Umbrella, Smile, Calendar, CheckCircle, CreditCard, 
  ChevronRight, Menu, X, Facebook, Instagram, Phone, Mail, 
  Info, ArrowRight, Loader2, User, Settings, ShieldCheck, 
  FileText, CalendarDays, Check, Banknote, ListOrdered, Shield,
  ArrowLeft, MapPin, AlertCircle, CalendarClock, ChevronLeft, Edit3, Save
} from 'lucide-react';

// --- MOCK DATA ---
const PRODUCTS = [
  {
    id: 'cargo',
    name: 'Rower Cargo (Longjohn)',
    description: 'Idealny do przewozu dzieci i towarów. Szybki, zwinny i pakowny.',
    basePrice: 100,
    image: 'https://images.unsplash.com/photo-1596700813876-0cd405e3ec30?auto=format&fit=crop&q=80&w=800',
    icon: <Bike className="w-6 h-6" />,
    bookedDates: ['2026-06-10', '2026-06-11', '2026-06-12', '2026-06-20'],
    addons: [
      { id: 'daszek', name: 'Daszek przeciwdeszczowy', price: 15, icon: <Umbrella className="w-4 h-4" /> },
      { id: 'poncho', name: 'Poncho dla kierującego', price: 10, icon: <CloudRain className="w-4 h-4" /> },
      { id: 'poduszki', name: 'Dodatkowe poduszki dla dzieci', price: 5, icon: <Smile className="w-4 h-4" /> }
    ]
  },
  {
    id: 'tandem',
    name: 'Rower Tandem',
    description: 'Podwójna radość z jazdy! Świetny na wycieczki we dwoje.',
    basePrice: 80,
    image: 'https://images.unsplash.com/photo-1517406692737-142f132e4823?auto=format&fit=crop&q=80&w=800',
    icon: <Users className="w-6 h-6" />,
    bookedDates: ['2026-06-05', '2026-06-06'],
    addons: []
  },
  {
    id: 'trailer-kids',
    name: 'Przyczepka dla 2 dzieci',
    description: 'Bezpieczna i wygodna przyczepka z pasami. Pasuje do większości rowerów.',
    basePrice: 45,
    image: 'https://images.unsplash.com/photo-1621245037947-f07a049f57eb?auto=format&fit=crop&q=80&w=800',
    icon: <Baby className="w-6 h-6" />,
    bookedDates: [],
    addons: []
  },
  {
    id: 'trailer-pedals',
    name: 'Przyczepka z pedałami (doczepka)',
    description: 'Dla starszego dziecka, które chce aktywnie uczestniczyć w pedałowaniu.',
    basePrice: 40,
    image: 'https://images.unsplash.com/photo-1471506480208-91b3a4cc78be?auto=format&fit=crop&q=80&w=800',
    icon: <Bike className="w-6 h-6" />,
    bookedDates: [],
    addons: []
  },
  {
    id: 'trailer-cargo',
    name: 'Przyczepka towarowa',
    description: 'Przewieź duże zakupy, sprzęt na biwak lub inne gabaryty.',
    basePrice: 30,
    image: 'https://images.unsplash.com/photo-1628744448840-55bdb2497bbf?auto=format&fit=crop&q=80&w=800',
    icon: <Box className="w-6 h-6" />,
    bookedDates: [],
    addons: []
  },
  {
    id: 'car-rack',
    name: 'Bagażnik samochodowy na 2 rowery',
    description: 'Montowany na hak holowniczy. Łatwy w obsłudze i bezpieczny.',
    basePrice: 35,
    image: 'https://images.unsplash.com/photo-1542362567-b07e54358753?auto=format&fit=crop&q=80&w=800',
    icon: <Car className="w-6 h-6" />,
    bookedDates: [],
    addons: []
  }
];

// --- MOCK DATA FOR ADMIN/USER PANELS ---
const MOCK_USERS = [
  { id: 'u1', name: 'Jan Kowalski', email: 'jan@example.com', phone: '500111222', address: 'Warszawska 1, Radzymin' },
  { id: 'u2', name: 'Anna Nowak', email: 'anna@example.com', phone: '600333444', address: 'Leśna 5/10, Marki' },
  { id: 'u3', name: 'Piotr Wiśniewski', email: 'piotr@example.com', phone: '700555666', address: 'Wierzbowa 2, Ząbki' }
];

const MOCK_TRANSFERS = [
  { id: 't1', date: '2026-06-04 10:15', sender: 'Jan Kowalski', title: 'CARGO-1234', amount: 150, status: 'nieprzypisany' },
  { id: 't2', date: '2026-06-03 14:20', sender: 'Anna Nowak', title: 'CARGO-9981', amount: 80, status: 'dopasowany', orderId: 'CARGO-9981' },
  { id: 't3', date: '2026-06-04 18:00', sender: 'Nieznany Nadawca', title: 'ZA ROWER', amount: 100, status: 'nieprzypisany' }
];

const MOCK_ORDERS = [
  { id: 'CARGO-1234', date: '2026-06-04', userId: 'u1', amount: 150, status: 'oczekuje na płatność', payment: 'blik', items: 'Rower Cargo (Longjohn)' },
  { id: 'CARGO-9981', date: '2026-06-03', userId: 'u2', amount: 80, status: 'opłacone', payment: 'blik', items: 'Rower Tandem' },
  { id: 'CARGO-5555', date: '2026-06-02', userId: 'u3', amount: 100, status: 'zaakceptowane', payment: 'gotówka', items: 'Rower Cargo + Daszek' }
];

export default function App() {
  const [currentView, setCurrentView] = useState('home'); // home, product, checkout, payment, success, user, admin
  const [selectedProduct, setSelectedProduct] = useState(null);
  const [cart, setCart] = useState([]);
  const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false);
  
  // User & Checkout Details
  const [userDetails, setUserDetails] = useState({ 
    name: '', email: '', phone: '', address: '', isAdult: false, acceptTos: false 
  });
  const [paymentMethod, setPaymentMethod] = useState('blik'); // blik, cash
  
  // Admin State
  const [adminOrders, setAdminOrders] = useState(MOCK_ORDERS);
  const [adminTransfers, setAdminTransfers] = useState(MOCK_TRANSFERS);
  const [adminUsers, setAdminUsers] = useState(MOCK_USERS);
  const [adminView, setAdminView] = useState('dashboard'); // dashboard, userDetail
  const [adminSelectedUserId, setAdminSelectedUserId] = useState(null);

  const [isLoggedIn, setIsLoggedIn] = useState(false);
  const [showAuthModal, setShowAuthModal] = useState(false);
  const [deleteAccountState, setDeleteAccountState] = useState('idle'); // idle, confirming, requested
  const [checkoutPassword, setCheckoutPassword] = useState('');

  // Calculate global totals from cart
  const { cartTotal, addonsTotal, finalTotal } = useMemo(() => {
    let base = 0;
    let addonsPrice = 0;
    
    cart.forEach(item => {
      base += (item.basePrice * item.rentalDays);
      if (item.selectedAddons) {
        item.selectedAddons.forEach(addon => {
          addonsPrice += (addon.price * item.rentalDays);
        });
      }
    });

    return {
      cartTotal: base,
      addonsTotal: addonsPrice,
      finalTotal: base + addonsPrice
    };
  }, [cart]);

  const navigateTo = (view, product = null) => {
    setCurrentView(view);
    if (product) setSelectedProduct(product);
    setIsMobileMenuOpen(false);
    window.scrollTo(0, 0);
  };

  const removeFromCart = (cartId) => {
    setCart(cart.filter(item => item.cartId !== cartId));
  };

  const markOrderPaid = (id) => {
    setAdminOrders(adminOrders.map(o => o.id === id ? { ...o, status: 'opłacone' } : o));
  };

  // --- SUBCOMPONENTS ---

  const Header = () => (
    <header className="sticky top-0 z-50 bg-white border-b border-gray-100 shadow-sm">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <div className="flex justify-between items-center h-20">
          <div className="flex-shrink-0 flex items-center cursor-pointer" onClick={() => navigateTo('home')}>
            <Bike className="h-8 w-8 text-emerald-600" />
            <span className="ml-2 text-2xl font-black tracking-tighter text-gray-900">
              cargo.<span className="text-emerald-600">mleczki</span>.pl
            </span>
          </div>

          {/* Desktop Nav */}
          <nav className="hidden md:flex space-x-6 items-center">
            <button onClick={() => navigateTo('home')} className="text-gray-600 hover:text-emerald-600 font-medium transition-colors">Oferta</button>
            <button onClick={() => isLoggedIn ? navigateTo('user') : setShowAuthModal(true)} className="text-gray-600 hover:text-emerald-600 font-medium transition-colors flex items-center"><User className="w-4 h-4 mr-1"/> {isLoggedIn ? 'Panel Klienta' : 'Zaloguj się'}</button>
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

          {/* Mobile menu button */}
          <div className="md:hidden flex items-center space-x-4">
            <div className="relative cursor-pointer" onClick={() => navigateTo('checkout')}>
              <ShoppingCart className="w-6 h-6 text-gray-700" />
              {cart.length > 0 && (
                <span className="absolute -top-2 -right-2 bg-red-500 text-white text-xs font-bold rounded-full w-5 h-5 flex items-center justify-center">
                  {cart.length}
                </span>
              )}
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
            <button onClick={() => { setIsMobileMenuOpen(false); isLoggedIn ? navigateTo('user') : setShowAuthModal(true); }} className="block w-full text-left px-3 py-3 text-base font-medium text-gray-700 hover:bg-emerald-50 rounded-lg">{isLoggedIn ? 'Panel Klienta' : 'Zaloguj się'}</button>
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

  const AuthModal = () => {
    if (!showAuthModal) return null;
    return (
      <div className="fixed inset-0 z-[100] flex items-center justify-center bg-gray-900/50 backdrop-blur-sm p-4">
        <div className="bg-white rounded-3xl shadow-2xl max-w-md w-full p-8 relative">
          <button onClick={() => setShowAuthModal(false)} className="absolute top-4 right-4 text-gray-400 hover:text-gray-900 transition-colors">
            <X className="w-6 h-6" />
          </button>
          <div className="w-12 h-12 bg-emerald-100 text-emerald-600 rounded-full flex items-center justify-center mb-4">
            <User className="w-6 h-6" />
          </div>
          <h2 className="text-2xl font-black text-gray-900 mb-2">Zaloguj się</h2>
          <p className="text-gray-500 text-sm mb-6">Wpisz swoje dane, aby uzyskać dostęp do historii rezerwacji i szybciej zamawiać bez wpisywania danych.</p>
          
          <div className="space-y-4 mb-6">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Adres e-mail</label>
              <input type="email" className="w-full p-3 border border-gray-300 rounded-xl focus:ring-2 focus:ring-emerald-500 outline-none" placeholder="jan@example.com" />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Hasło</label>
              <input type="password" className="w-full p-3 border border-gray-300 rounded-xl focus:ring-2 focus:ring-emerald-500 outline-none" placeholder="••••••••" />
            </div>
          </div>
          <button 
            onClick={() => {
              setIsLoggedIn(true);
              setShowAuthModal(false);
              // Mock load user data
              setUserDetails({
                name: 'Jan Kowalski', 
                email: 'jan@example.com', 
                phone: '500 111 222', 
                address: 'Warszawska 1, Radzymin',
                isAdult: true,
                acceptTos: true
              });
            }}
            className="w-full bg-emerald-600 hover:bg-emerald-500 text-white font-bold py-3 px-4 rounded-xl transition-all shadow-md"
          >
            Zaloguj się
          </button>
        </div>
      </div>
    );
  };

  const ProductCard = ({ product }) => (
    <div className="bg-white rounded-2xl shadow-sm border border-gray-100 overflow-hidden hover:shadow-xl transition-shadow duration-300 flex flex-col cursor-pointer" onClick={() => navigateTo('product', product)}>
      <div className="h-56 overflow-hidden relative group">
        <img src={product.image} alt={product.name} className="w-full h-full object-cover group-hover:scale-105 transition-transform duration-500" />
        <div className="absolute top-4 left-4 bg-white/90 backdrop-blur px-3 py-1.5 rounded-full flex items-center space-x-2 shadow-sm">
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
    const [itemStartDate, setItemStartDate] = useState(null);
    const [itemEndDate, setItemEndDate] = useState(null);
    const [selectedAddons, setSelectedAddons] = useState([]);
    const [currentMonthDate, setCurrentMonthDate] = useState(new Date(2026, 5, 1)); // Context: June 2026

    // --- CALENDAR LOGIC ---
    const getDaysInMonth = (year, month) => new Date(year, month + 1, 0).getDate();
    const getFirstDayOfMonth = (year, month) => new Date(year, month, 1).getDay(); // 0 is Sunday
    
    const generateCalendarGrid = () => {
      const year = currentMonthDate.getFullYear();
      const month = currentMonthDate.getMonth();
      const daysInMonth = getDaysInMonth(year, month);
      let firstDay = getFirstDayOfMonth(year, month);
      firstDay = firstDay === 0 ? 6 : firstDay - 1; // Make Monday = 0
      
      const grid = [];
      let dayCounter = 1;
      
      // Calculate previous month's trailing days
      const prevMonthDays = getDaysInMonth(year, month - 1);
      
      for (let i = 0; i < 6; i++) {
        const row = [];
        for (let j = 0; j < 7; j++) {
          if (i === 0 && j < firstDay) {
            row.push({ empty: true, day: prevMonthDays - firstDay + j + 1 });
          } else if (dayCounter > daysInMonth) {
            row.push({ empty: true, day: dayCounter - daysInMonth });
            dayCounter++;
          } else {
            const dateStr = `${year}-${String(month + 1).padStart(2, '0')}-${String(dayCounter).padStart(2, '0')}`;
            const isBooked = selectedProduct.bookedDates?.includes(dateStr);
            
            // Check selections
            let isSelected = false;
            let isBetween = false;
            
            const currentDate = new Date(dateStr).getTime();
            const start = itemStartDate ? new Date(itemStartDate).getTime() : null;
            const end = itemEndDate ? new Date(itemEndDate).getTime() : null;

            if (start && currentDate === start) isSelected = true;
            if (end && currentDate === end) isSelected = true;
            if (start && end && currentDate > start && currentDate < end) isBetween = true;

            const isPast = currentDate < new Date(new Date().setHours(0,0,0,0)).getTime();

            row.push({ 
              empty: false, 
              day: dayCounter, 
              dateStr, 
              isBooked, 
              isSelected, 
              isBetween,
              isPast
            });
            dayCounter++;
          }
        }
        grid.push(row);
        if (dayCounter > daysInMonth && row.length === 7) break;
      }
      return grid;
    };

    const handleDayClick = (day) => {
      if (day.empty || day.isBooked || day.isPast) return;

      const clickedTime = new Date(day.dateStr).getTime();

      // Reset if both are set or if clicking before start
      if ((itemStartDate && itemEndDate) || (itemStartDate && clickedTime < new Date(itemStartDate).getTime())) {
        setItemStartDate(day.dateStr);
        setItemEndDate(null);
        return;
      }

      if (!itemStartDate) {
        setItemStartDate(day.dateStr);
      } else if (!itemEndDate) {
        // Check if there are booked dates in between the selection
        const startTime = new Date(itemStartDate).getTime();
        let hasBookedBetween = false;
        
        selectedProduct.bookedDates?.forEach(bookedDate => {
          const bookedTime = new Date(bookedDate).getTime();
          if (bookedTime > startTime && bookedTime < clickedTime) {
            hasBookedBetween = true;
          }
        });

        if (hasBookedBetween) {
          alert("Wybrany zakres zawiera dni, w których sprzęt jest już zarezerwowany. Wybierz inny, ciągły termin.");
          setItemStartDate(day.dateStr); // Reset to new start
        } else {
          setItemEndDate(day.dateStr);
        }
      }
    };

    const nextMonth = () => setCurrentMonthDate(new Date(currentMonthDate.getFullYear(), currentMonthDate.getMonth() + 1, 1));
    const prevMonth = () => setCurrentMonthDate(new Date(currentMonthDate.getFullYear(), currentMonthDate.getMonth() - 1, 1));

    const monthNames = ["Styczeń", "Luty", "Marzec", "Kwiecień", "Maj", "Czerwiec", "Lipiec", "Sierpień", "Wrzesień", "Październik", "Listopad", "Grudzień"];
    // --- END CALENDAR LOGIC ---

    const itemRentalDays = useMemo(() => {
      if (!itemStartDate) return 0;
      if (!itemEndDate) return 1;
      const start = new Date(itemStartDate);
      const end = new Date(itemEndDate);
      const diffTime = Math.abs(end - start);
      const diffDays = Math.ceil(diffTime / (1000 * 60 * 60 * 24)); 
      return diffDays >= 0 ? diffDays + 1 : 1;
    }, [itemStartDate, itemEndDate]);

    const itemTotal = useMemo(() => {
      if (itemRentalDays === 0) return 0;
      let addonsPrice = selectedAddons.reduce((acc, curr) => acc + curr.price, 0);
      return (selectedProduct.basePrice + addonsPrice) * itemRentalDays;
    }, [selectedProduct, selectedAddons, itemRentalDays]);

    const handleAddToCart = () => {
      if (!itemStartDate) {
        alert("Proszę wybrać datę wynajmu na kalendarzu dostępności.");
        return;
      }
      const end = itemEndDate || itemStartDate;
      
      setCart([...cart, { 
        ...selectedProduct, 
        cartId: Date.now(), 
        selectedAddons,
        startDate: itemStartDate,
        endDate: end,
        rentalDays: itemRentalDays === 0 ? 1 : itemRentalDays
      }]);
      navigateTo('checkout');
    };

    const toggleAddon = (addon) => {
      if (selectedAddons.find(a => a.id === addon.id)) {
        setSelectedAddons(selectedAddons.filter(a => a.id !== addon.id));
      } else {
        setSelectedAddons([...selectedAddons, addon]);
      }
    };

    return (
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-12">
        <button onClick={() => navigateTo('home')} className="flex items-center text-gray-500 hover:text-emerald-600 mb-8 transition-colors">
          <ArrowLeft className="w-5 h-5 mr-2" /> Wróć do oferty
        </button>

        <div className="grid grid-cols-1 lg:grid-cols-2 gap-12">
          {/* Left: Image */}
          <div className="rounded-3xl overflow-hidden shadow-lg h-96 lg:h-[600px] sticky top-24">
            <img src={selectedProduct.image} alt={selectedProduct.name} className="w-full h-full object-cover" />
          </div>

          {/* Right: Details & Booking */}
          <div className="flex flex-col">
            <div className="flex items-center space-x-3 mb-4">
              <div className="p-3 bg-emerald-100 text-emerald-600 rounded-xl">{selectedProduct.icon}</div>
              <h1 className="text-3xl lg:text-4xl font-black text-gray-900">{selectedProduct.name}</h1>
            </div>
            
            <p className="text-lg text-gray-600 mb-8 leading-relaxed">
              {selectedProduct.description}
            </p>

            {/* Interactive Calendar Widget */}
            <div className="bg-gray-50 border border-gray-200 rounded-2xl p-6 mb-8">
              <h3 className="text-lg font-bold text-gray-900 mb-4 flex items-center">
                <CalendarClock className="w-5 h-5 mr-2 text-emerald-600" /> Dostępność i rezerwacja
              </h3>
              <p className="text-sm text-gray-500 mb-4">Zaznacz dzień odbioru i zwrotu. Wyszarzone dni są już zarezerwowane.</p>
              
              <div className="bg-white rounded-xl border border-gray-200 overflow-hidden select-none">
                <div className="flex items-center justify-between p-4 border-b border-gray-100 bg-gray-50/50">
                  <button onClick={prevMonth} className="p-1 hover:bg-gray-200 rounded text-gray-600"><ChevronLeft className="w-5 h-5"/></button>
                  <span className="font-bold text-gray-900">{monthNames[currentMonthDate.getMonth()]} {currentMonthDate.getFullYear()}</span>
                  <button onClick={nextMonth} className="p-1 hover:bg-gray-200 rounded text-gray-600"><ChevronRight className="w-5 h-5"/></button>
                </div>
                <div className="p-4">
                  <div className="grid grid-cols-7 gap-1 text-center text-xs font-medium text-gray-500 mb-2">
                    <div>Pn</div><div>Wt</div><div>Śr</div><div>Cz</div><div>Pt</div><div>Sb</div><div>Nd</div>
                  </div>
                  <div className="grid grid-cols-7 gap-1">
                    {generateCalendarGrid().map((row, i) => 
                      row.map((day, j) => {
                        let baseClasses = "h-10 w-full flex items-center justify-center text-sm rounded-lg transition-colors ";
                        if (day.empty) {
                          baseClasses += "text-transparent pointer-events-none";
                        } else if (day.isPast) {
                          baseClasses += "text-gray-300 bg-gray-50 cursor-not-allowed";
                        } else if (day.isBooked) {
                          baseClasses += "bg-gray-100 text-gray-400 cursor-not-allowed line-through decoration-gray-400";
                        } else if (day.isSelected) {
                          baseClasses += "bg-emerald-600 text-white font-bold shadow-md cursor-pointer";
                        } else if (day.isBetween) {
                          baseClasses += "bg-emerald-100 text-emerald-800 cursor-pointer";
                        } else {
                          baseClasses += "hover:bg-gray-100 text-gray-700 cursor-pointer";
                        }
                        return (
                          <div 
                            key={`${i}-${j}`} 
                            className={baseClasses}
                            onClick={() => handleDayClick(day)}
                          >
                            {!day.empty && day.day}
                          </div>
                        )
                      })
                    )}
                  </div>
                </div>
              </div>
              
              <div className="mt-4 flex space-x-4 text-xs text-gray-500">
                <div className="flex items-center"><span className="w-3 h-3 bg-emerald-600 rounded-sm mr-1"></span> Wybrane</div>
                <div className="flex items-center"><span className="w-3 h-3 bg-gray-100 border border-gray-200 rounded-sm mr-1"></span> Zajęte</div>
              </div>

              {itemStartDate && (
                 <div className="mt-4 p-3 bg-emerald-50 text-emerald-800 rounded-lg flex items-center text-sm font-medium">
                   <CheckCircle className="w-4 h-4 mr-2" />
                   Wybrano termin: od {itemStartDate} do {itemEndDate || itemStartDate} ({itemRentalDays || 1} dni)
                 </div>
              )}
            </div>

            {/* Addons */}
            {selectedProduct.addons && selectedProduct.addons.length > 0 && (
              <div className="mb-8">
                <h3 className="text-lg font-bold text-gray-900 mb-4">Opcje dodatkowe</h3>
                <div className="space-y-3">
                  {selectedProduct.addons.map(addon => {
                    const isSelected = selectedAddons.find(a => a.id === addon.id);
                    return (
                      <label key={addon.id} className={`flex items-center justify-between p-4 rounded-xl cursor-pointer transition-colors border ${isSelected ? 'bg-emerald-50 border-emerald-200' : 'bg-white border-gray-200 hover:border-emerald-300'}`}>
                        <div className="flex items-center space-x-4">
                          <input 
                            type="checkbox" 
                            checked={!!isSelected}
                            onChange={() => toggleAddon(addon)}
                            className="w-5 h-5 text-emerald-600 rounded border-gray-300 focus:ring-emerald-500"
                          />
                          <div className="flex items-center space-x-2">
                            <div className="text-gray-500">{addon.icon}</div>
                            <span className="font-medium text-gray-900">{addon.name}</span>
                          </div>
                        </div>
                        <span className="font-semibold text-emerald-700">+{addon.price} zł / dobę</span>
                      </label>
                    )
                  })}
                </div>
              </div>
            )}

            {/* Price & Action */}
            <div className="mt-auto bg-gray-900 rounded-2xl p-6 text-white flex flex-col sm:flex-row items-center justify-between">
              <div className="mb-4 sm:mb-0 text-center sm:text-left">
                <p className="text-gray-400 text-sm">Cena za wybrany okres:</p>
                <p className="text-3xl font-black text-emerald-400">{itemTotal > 0 ? itemTotal : selectedProduct.basePrice} zł</p>
              </div>
              <button 
                onClick={handleAddToCart}
                className={`w-full sm:w-auto font-bold py-4 px-8 rounded-xl transition-colors duration-300 flex items-center justify-center space-x-2 ${itemStartDate ? 'bg-emerald-500 hover:bg-emerald-400 text-white' : 'bg-gray-700 text-gray-400 cursor-not-allowed'}`}
              >
                <ShoppingCart className="w-5 h-5" />
                <span>Dodaj do rezerwacji</span>
              </button>
            </div>
            <p className="text-xs text-gray-500 text-center mt-4">Płatność bezpiecznym przelewem BLIK lub gotówką przy odbiorze.</p>
          </div>
        </div>
      </div>
    );
  };

  const CheckoutView = () => {
    if (cart.length === 0) {
      return (
        <div className="max-w-3xl mx-auto px-4 py-20 text-center">
          <div className="bg-gray-50 rounded-full w-24 h-24 flex items-center justify-center mx-auto mb-6">
            <ShoppingCart className="w-12 h-12 text-gray-400" />
          </div>
          <h2 className="text-2xl font-bold text-gray-900 mb-4">Twój koszyk jest pusty</h2>
          <button onClick={() => navigateTo('home')} className="bg-emerald-600 hover:bg-emerald-700 text-white px-8 py-3 rounded-xl font-semibold transition-colors">Wróć do oferty</button>
        </div>
      );
    }

    return (
      <div className="max-w-7xl mx-auto px-4 py-12 lg:flex lg:space-x-12">
        <div className="lg:w-2/3">
          <h2 className="text-3xl font-black text-gray-900 mb-8">Dane do umowy i zamówienie</h2>

          <div className="space-y-4 mb-8">
            <h3 className="text-xl font-bold text-gray-900">Sprzęt w koszyku</h3>
            {cart.map((item) => (
              <div key={item.cartId} className="bg-white rounded-2xl shadow-sm border border-gray-100 p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4">
                <div className="flex items-center space-x-4">
                  <div className="w-16 h-16 bg-gray-50 rounded-xl flex items-center justify-center text-gray-400">
                    {item.icon}
                  </div>
                  <div>
                    <h4 className="font-bold text-gray-900">{item.name}</h4>
                    <p className="text-sm text-emerald-600 font-medium mt-1">Od {item.startDate} do {item.endDate} ({item.rentalDays} dni)</p>
                    {item.selectedAddons && item.selectedAddons.length > 0 && (
                      <div className="mt-2 space-y-1">
                        {item.selectedAddons.map(a => (
                          <p key={a.id} className="text-xs text-gray-500">• {a.name}</p>
                        ))}
                      </div>
                    )}
                  </div>
                </div>
                <div className="text-right flex flex-col items-end">
                  <p className="font-bold text-lg text-gray-900">
                    {(item.basePrice + (item.selectedAddons?.reduce((acc, a) => acc + a.price, 0) || 0)) * item.rentalDays} zł
                  </p>
                  <button onClick={() => removeFromCart(item.cartId)} className="text-sm text-red-500 mt-2 hover:underline">Usuń</button>
                </div>
              </div>
            ))}
          </div>

          <div className="bg-white rounded-2xl shadow-sm border border-gray-100 p-6 mb-8">
            <div className="flex flex-col sm:flex-row sm:items-center justify-between mb-4">
              <h3 className="text-xl font-bold text-gray-900 flex items-center">
                <FileText className="w-5 h-5 mr-2 text-emerald-600" /> Dane Najemcy (do umowy)
              </h3>
              {!isLoggedIn && (
                <button onClick={() => setShowAuthModal(true)} className="text-sm text-emerald-600 font-medium hover:underline mt-2 sm:mt-0">
                  Masz już konto? Zaloguj się
                </button>
              )}
            </div>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-6">
              <div className="md:col-span-2">
                <label className="block text-sm font-medium text-gray-700 mb-1">Imię i nazwisko</label>
                <input type="text" value={userDetails.name} onChange={(e) => setUserDetails({...userDetails, name: e.target.value})} className="w-full p-3 border border-gray-300 rounded-xl focus:ring-2 focus:ring-emerald-500" placeholder="Jan Kowalski" />
              </div>
              <div className="md:col-span-2">
                <label className="block text-sm font-medium text-gray-700 mb-1">Adres zamieszkania</label>
                <input type="text" value={userDetails.address} onChange={(e) => setUserDetails({...userDetails, address: e.target.value})} className="w-full p-3 border border-gray-300 rounded-xl focus:ring-2 focus:ring-emerald-500" placeholder="ul. Przykładowa 12/3, 05-250 Radzymin" />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Adres e-mail</label>
                <input type="email" value={userDetails.email} onChange={(e) => setUserDetails({...userDetails, email: e.target.value})} className="w-full p-3 border border-gray-300 rounded-xl focus:ring-2 focus:ring-emerald-500" placeholder="jan@example.com" />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Telefon</label>
                <input type="tel" value={userDetails.phone} onChange={(e) => setUserDetails({...userDetails, phone: e.target.value})} className="w-full p-3 border border-gray-300 rounded-xl focus:ring-2 focus:ring-emerald-500" placeholder="123 456 789" />
              </div>
              
              {!isLoggedIn && (
                <div className="md:col-span-2 mt-2 p-5 bg-emerald-50/50 rounded-xl border border-emerald-100">
                  <h4 className="font-bold text-gray-900 mb-1">Utworzenie konta</h4>
                  <p className="text-sm text-gray-500 mb-4">Przy tym zamówieniu automatycznie utworzymy dla Ciebie konto, aby ułatwić kolejne rezerwacje (zgodnie z regulaminem).</p>
                  <label className="block text-sm font-medium text-gray-700 mb-1">Hasło do nowego konta</label>
                  <input type="password" value={checkoutPassword} onChange={(e) => setCheckoutPassword(e.target.value)} className="w-full p-3 border border-gray-300 rounded-xl focus:ring-2 focus:ring-emerald-500 bg-white" placeholder="Ustal bezpieczne hasło..." />
                </div>
              )}
            </div>

            <div className="space-y-3 border-t border-gray-100 pt-6">
              <label className="flex items-start space-x-3 cursor-pointer">
                <input type="checkbox" checked={userDetails.isAdult} onChange={(e) => setUserDetails({...userDetails, isAdult: e.target.checked})} className="mt-1 w-5 h-5 text-emerald-600 rounded border-gray-300 focus:ring-emerald-500" />
                <span className="text-sm text-gray-700">Potwierdzam, że jestem osobą pełnoletnią i posiadam dokument tożsamości do wglądu przy odbiorze sprzętu.*</span>
              </label>
              <label className="flex items-start space-x-3 cursor-pointer">
                <input type="checkbox" checked={userDetails.acceptTos} onChange={(e) => setUserDetails({...userDetails, acceptTos: e.target.checked})} className="mt-1 w-5 h-5 text-emerald-600 rounded border-gray-300 focus:ring-emerald-500" />
                <span className="text-sm text-gray-700">Akceptuję <a href="#" className="text-emerald-600 underline">Regulamin i Umowę Najmu</a>. Wyrażam zgodę na przetwarzanie danych osobowych w celu realizacji zamówienia (zgodnie z RODO).*</span>
              </label>
            </div>
          </div>

          <div className="bg-white rounded-2xl shadow-sm border border-gray-100 p-6">
            <h3 className="text-xl font-bold text-gray-900 mb-4">Metoda płatności</h3>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <label className={`flex flex-col items-center p-6 border-2 rounded-2xl cursor-pointer transition-all ${paymentMethod === 'blik' ? 'border-emerald-500 bg-emerald-50' : 'border-gray-200 hover:border-emerald-300'}`}>
                <input type="radio" name="payment" className="hidden" checked={paymentMethod === 'blik'} onChange={() => setPaymentMethod('blik')} />
                <CreditCard className={`w-8 h-8 mb-2 ${paymentMethod === 'blik' ? 'text-emerald-600' : 'text-gray-400'}`} />
                <span className="font-bold text-gray-900">Szybki Przelew BLIK</span>
                <span className="text-xs text-center mt-1 text-gray-500">Bezpiecznie i bez prowizji (autorski system).</span>
              </label>
              
              <label className={`flex flex-col items-center p-6 border-2 rounded-2xl cursor-pointer transition-all ${paymentMethod === 'cash' ? 'border-emerald-500 bg-emerald-50' : 'border-gray-200 hover:border-emerald-300'}`}>
                <input type="radio" name="payment" className="hidden" checked={paymentMethod === 'cash'} onChange={() => setPaymentMethod('cash')} />
                <Banknote className={`w-8 h-8 mb-2 ${paymentMethod === 'cash' ? 'text-emerald-600' : 'text-gray-400'}`} />
                <span className="font-bold text-gray-900">Gotówka przy odbiorze</span>
                <span className="text-xs text-center mt-1 text-gray-500">Płatność na miejscu (odliczona kwota).</span>
              </label>
            </div>
          </div>
        </div>

        <div className="lg:w-1/3 mt-8 lg:mt-0">
          <div className="bg-gray-900 rounded-3xl p-8 text-white sticky top-24 shadow-2xl">
            <h3 className="text-2xl font-bold mb-6">Podsumowanie</h3>
            <div className="space-y-4 mb-6 text-gray-300">
              <div className="flex justify-between">
                <span>Koszt sprzętu</span>
                <span className="font-medium text-white">{cartTotal} zł</span>
              </div>
              {addonsTotal > 0 && (
                <div className="flex justify-between">
                  <span>Opcje dodatkowe</span>
                  <span className="font-medium text-white">{addonsTotal} zł</span>
                </div>
              )}
              <div className="border-t border-gray-700 my-4"></div>
              <div className="flex justify-between text-xl font-bold text-white">
                <span>Razem do zapłaty</span>
                <span className="text-emerald-400">{finalTotal} zł</span>
              </div>
            </div>

            <button 
              onClick={() => {
                if (!userDetails.name || !userDetails.email || !userDetails.phone || !userDetails.address) {
                  alert('Wypełnij wszystkie dane kontaktowe.'); return;
                }
                if (!isLoggedIn && !checkoutPassword) {
                  alert('Podaj hasło, abyśmy mogli utworzyć dla Ciebie konto.'); return;
                }
                if (!userDetails.isAdult || !userDetails.acceptTos) {
                  alert('Musisz potwierdzić pełnoletność oraz zaakceptować regulamin, aby wypożyczyć sprzęt.'); return;
                }
                navigateTo('payment');
              }}
              className="w-full bg-emerald-500 hover:bg-emerald-400 text-white font-bold py-4 px-4 rounded-xl transition-colors duration-300 flex items-center justify-center"
            >
              Potwierdzam zamówienie <ChevronRight className="ml-2 w-5 h-5" />
            </button>
          </div>
        </div>
      </div>
    );
  };

  const PaymentView = () => {
    const [isVerifying, setIsVerifying] = useState(false);

    const handleSimulatePayment = () => {
      if (!isLoggedIn) {
        setIsLoggedIn(true); // Account created successfully during payment
      }

      if (paymentMethod === 'cash') {
        navigateTo('success');
        return;
      }
      setIsVerifying(true);
      setTimeout(() => { setIsVerifying(false); navigateTo('success'); }, 4000);
    };

    if (paymentMethod === 'cash') {
      return (
        <div className="max-w-2xl mx-auto px-4 py-16 text-center">
          <Banknote className="w-20 h-20 text-emerald-600 mx-auto mb-6" />
          <h2 className="text-4xl font-black mb-4">Płatność gotówką</h2>
          <p className="text-lg text-gray-600 mb-8">Wybrałeś płatność gotówką na miejscu. Przygotuj prosimy odliczoną kwotę ({finalTotal} zł) przy odbiorze sprzętu.</p>
          <button onClick={handleSimulatePayment} className="bg-emerald-600 hover:bg-emerald-700 text-white px-8 py-4 rounded-xl font-bold text-lg shadow-lg">
            Rozumiem, zakończ rezerwację
          </button>
        </div>
      );
    }

    return (
      <div className="max-w-2xl mx-auto px-4 py-16">
        <div className="bg-white rounded-3xl shadow-xl border border-gray-100 overflow-hidden text-center">
          <div className="bg-gray-900 text-white p-8">
            <CreditCard className="w-16 h-16 mx-auto mb-4 text-emerald-400" />
            <h2 className="text-3xl font-black mb-2">Płatność BLIK na telefon</h2>
          </div>
          <div className="p-8">
            <div className="bg-emerald-50 border-2 border-emerald-100 rounded-2xl p-6 mb-8 text-left">
              <h3 className="font-bold text-gray-900 mb-4 flex items-center"><Info className="w-5 h-5 mr-2 text-emerald-600" /> Instrukcja</h3>
              <ol className="list-decimal list-inside space-y-4 text-gray-700">
                <li>Zaloguj się do aplikacji banku i wybierz "Przelew BLIK na telefon".</li>
                <li>Odbiorca: <span className="font-black text-gray-900">500 123 456</span></li>
                <li>Kwota: <span className="font-black text-emerald-700">{finalTotal} zł</span></li>
                <li>Tytuł (BARDZO WAŻNE):<br/><span className="inline-block bg-white border border-gray-200 px-4 py-2 mt-2 rounded-lg font-mono text-emerald-700 font-bold">CARGO-{Math.floor(Math.random()*10000)}</span></li>
              </ol>
            </div>
            {isVerifying ? (
              <div className="flex flex-col items-center justify-center py-4">
                <Loader2 className="w-10 h-10 text-emerald-600 animate-spin mb-4" />
                <p className="font-bold text-gray-900">Nasłuchujemy na przelew z banku...</p>
              </div>
            ) : (
              <button onClick={handleSimulatePayment} className="w-full bg-emerald-600 hover:bg-emerald-700 text-white font-bold py-4 px-6 rounded-xl transition-all shadow-lg text-lg">
                Wysłałem przelew BLIK
              </button>
            )}
          </div>
        </div>
      </div>
    );
  };

  const SuccessView = () => (
    <div className="max-w-2xl mx-auto px-4 py-20 text-center">
      <div className="bg-emerald-100 w-24 h-24 rounded-full flex items-center justify-center mx-auto mb-6">
        <CheckCircle className="w-12 h-12 text-emerald-600" />
      </div>
      <h2 className="text-4xl font-black text-gray-900 mb-4">Zrobione! Do zobaczenia.</h2>
      <p className="text-lg text-gray-600 mb-8">
        Szczegóły wysłaliśmy na e-mail: <strong>{userDetails.email}</strong>. Pamiętaj o zabraniu dokumentu tożsamości przy odbiorze.
      </p>
      <button onClick={() => { setCart([]); navigateTo('user'); }} className="bg-gray-900 hover:bg-gray-800 text-white px-8 py-3 rounded-xl font-semibold transition-colors">
        Przejdź do Twojego panelu
      </button>
    </div>
  );

  const UserPanelView = () => (
    <div className="max-w-5xl mx-auto px-4 py-12">
      <h2 className="text-3xl font-black text-gray-900 mb-8">Panel Klienta</h2>
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
        <div className="lg:col-span-1">
          <div className="bg-white rounded-2xl border border-gray-100 p-6 shadow-sm">
            <div className="w-16 h-16 bg-emerald-100 rounded-full flex items-center justify-center mb-4 text-emerald-600">
              <User className="w-8 h-8" />
            </div>
            <h3 className="font-bold text-xl mb-1">{userDetails.name || 'Michał (Demo Klient)'}</h3>
            <p className="text-gray-500 text-sm mb-4">{userDetails.email || 'michal@example.com'}</p>
            <div className="border-t border-gray-100 pt-4 text-sm text-gray-600 space-y-2">
              <p><strong>Tel:</strong> {userDetails.phone || '---'}</p>
              <p><strong>Adres:</strong> {userDetails.address || '---'}</p>
            </div>

            <div className="border-t border-gray-100 pt-4 mt-4">
              {deleteAccountState === 'idle' && (
                <button onClick={() => setDeleteAccountState('confirming')} className="text-red-500 text-sm hover:underline font-medium">
                  Zażądaj usunięcia konta (RODO)
                </button>
              )}
              {deleteAccountState === 'confirming' && (
                <div className="bg-red-50 p-4 rounded-xl border border-red-100">
                  <h4 className="font-bold text-red-900 mb-1">Czy na pewno?</h4>
                  <p className="text-xs text-red-700 mb-3 leading-relaxed">
                    Wysyłasz żądanie usunięcia wszystkich swoich danych osobowych oraz historii zamówień z naszego systemu. Tej operacji nie można cofnąć.
                  </p>
                  <div className="flex space-x-2">
                    <button onClick={() => setDeleteAccountState('requested')} className="bg-red-600 text-white px-4 py-2 rounded-lg text-sm font-bold hover:bg-red-700 transition-colors">Tak, usuń konto</button>
                    <button onClick={() => setDeleteAccountState('idle')} className="bg-white border border-red-200 text-red-800 px-4 py-2 rounded-lg text-sm font-bold hover:bg-red-50 transition-colors">Anuluj</button>
                  </div>
                </div>
              )}
              {deleteAccountState === 'requested' && (
                <div className="bg-emerald-50 text-emerald-800 p-4 rounded-xl text-sm flex items-center font-medium border border-emerald-100">
                  <CheckCircle className="w-5 h-5 mr-2 flex-shrink-0" />
                  Wysłano prośbę o usunięcie. Administrator przetworzy ją w ciągu 14 dni.
                </div>
              )}
            </div>
          </div>
        </div>
        <div className="lg:col-span-2 space-y-4">
          <h3 className="font-bold text-xl text-gray-900 mb-4 flex items-center"><ListOrdered className="w-5 h-5 mr-2" /> Twoje rezerwacje</h3>
          {/* Mock recent order if cart was just processed */}
          <div className="bg-white rounded-2xl border border-emerald-200 p-6 shadow-sm">
            <div className="flex justify-between items-start mb-4">
              <div>
                <span className="bg-emerald-100 text-emerald-800 text-xs font-bold px-2 py-1 rounded">Aktywna</span>
                <h4 className="font-bold text-lg mt-2">CARGO-TEST</h4>
                <p className="text-sm text-gray-500">Najbliższy wynajem</p>
              </div>
              <div className="text-right">
                <p className="font-bold text-gray-900">{finalTotal > 0 ? finalTotal : 150} zł</p>
                <p className="text-xs text-gray-500">{paymentMethod === 'blik' ? 'Opłacone (BLIK)' : 'Do zapłaty gotówką'}</p>
              </div>
            </div>
          </div>
          <div className="bg-gray-50 rounded-2xl border border-gray-200 p-6 opacity-70">
            <div className="flex justify-between items-start">
              <div>
                <span className="bg-gray-200 text-gray-600 text-xs font-bold px-2 py-1 rounded">Zakończona</span>
                <h4 className="font-bold text-lg mt-2">CARGO-0012</h4>
                <p className="text-sm text-gray-500">Lipiec 2025</p>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );

  const AdminPanelView = () => {
    
    // Zmiana statusu przez Admina
    const linkTransferToOrder = (transferId, orderId) => {
      if (!orderId) return;
      setAdminTransfers(adminTransfers.map(t => t.id === transferId ? { ...t, status: 'dopasowany', orderId } : t));
      setAdminOrders(adminOrders.map(o => o.id === orderId ? { ...o, status: 'opłacone' } : o));
    };

    const getUserName = (userId) => {
      const u = adminUsers.find(user => user.id === userId);
      return u ? u.name : 'Nieznany Klient';
    };

    const handleUserClick = (userId) => {
      setAdminSelectedUserId(userId);
      setAdminView('userDetail');
    };

    // WIDOK 1: DASHBOARD
    if (adminView === 'dashboard') {
      return (
        <div className="max-w-7xl mx-auto px-4 py-12">
          <div className="flex items-center justify-between mb-8">
            <h2 className="text-3xl font-black text-gray-900 flex items-center"><Settings className="w-8 h-8 mr-3 text-red-500"/> Panel Administratora</h2>
          </div>

          <div className="bg-blue-50 border border-blue-200 text-blue-800 p-4 rounded-xl mb-8 flex text-sm shadow-sm">
            <Info className="w-5 h-5 mr-2 flex-shrink-0" />
            <p><strong>Baza CRM:</strong> W poniższych zamówieniach nazwy klientów są klikalne. Przejdź w profil użytkownika, aby edytować jego dane kontaktowe lub sprawdzić pełną historię.</p>
          </div>

          <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
            {/* Lista Zamówień */}
            <div className="bg-white rounded-2xl shadow-sm border border-gray-200 p-6">
              <h3 className="font-bold text-xl mb-6 border-b pb-2">Ostatnie zamówienia</h3>
              <div className="space-y-4">
                {adminOrders.map(order => (
                  <div key={order.id} className="border border-gray-100 bg-gray-50 p-4 rounded-xl">
                    <div className="flex justify-between items-center mb-2">
                      <span className="font-bold text-gray-900">{order.id}</span>
                      <span className={`text-xs px-2 py-1 rounded font-bold ${order.status === 'opłacone' ? 'bg-emerald-100 text-emerald-700' : 'bg-yellow-100 text-yellow-700'}`}>
                        {order.status.toUpperCase()}
                      </span>
                    </div>
                    <p className="text-sm text-gray-600 mb-1 flex items-center">
                      <User className="w-3 h-3 mr-1 text-emerald-600"/> 
                      <button onClick={() => handleUserClick(order.userId)} className="text-emerald-700 hover:underline font-semibold ml-1">
                        {getUserName(order.userId)}
                      </button>
                    </p>
                    <p className="text-sm text-gray-600 mb-3 flex items-center"><Bike className="w-3 h-3 mr-1"/> <span className="ml-1">{order.items}</span></p>
                    
                    <div className="flex justify-between items-center mt-2 pt-2 border-t border-gray-200">
                      <span className="font-bold text-gray-900">{order.amount} zł ({order.payment})</span>
                      {order.status !== 'opłacone' && order.payment === 'gotówka' && (
                        <button onClick={() => markOrderPaid(order.id)} className="text-xs bg-emerald-600 text-white px-3 py-1.5 rounded-lg hover:bg-emerald-700 shadow-sm transition-colors">
                          Oznacz zapłacone (Gotówka)
                        </button>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            </div>

            {/* Transfery bankowe - Parowanie */}
            <div className="bg-white rounded-2xl shadow-sm border border-gray-200 p-6">
              <h3 className="font-bold text-xl mb-6 border-b pb-2 flex items-center justify-between">
                <span>Skrzynka wpłat (BLIK)</span>
                <span className="text-xs font-normal bg-gray-100 px-2 py-1 rounded-md text-gray-600 border border-gray-200">Sync: Teraz</span>
              </h3>
              <div className="space-y-4">
                {adminTransfers.map(t => (
                  <div key={t.id} className={`border-l-4 p-4 rounded-r-xl ${t.status === 'dopasowany' ? 'border-emerald-500 bg-emerald-50/50' : 'border-red-400 bg-gray-50'}`}>
                    <div className="flex justify-between">
                      <span className="text-xs text-gray-500 font-medium">{t.date}</span>
                      <span className={`text-xs font-black tracking-wider ${t.status === 'dopasowany' ? 'text-emerald-600' : 'text-red-500'}`}>
                        {t.status === 'dopasowany' ? 'DOPASOWANY' : 'BRAK DOPASOWANIA'}
                      </span>
                    </div>
                    <p className="font-black text-lg text-gray-900 mt-1">{t.amount} PLN</p>
                    <p className="text-sm text-gray-600">Od: <span className="font-medium text-gray-800">{t.sender}</span></p>
                    <p className="text-sm font-mono text-blue-700 bg-blue-100/50 inline-block px-2 py-0.5 rounded mt-1">Tytuł: {t.title}</p>
                    
                    {/* Akcja: Łączenie z zamówieniem */}
                    {t.status !== 'dopasowany' && (
                      <div className="mt-3 pt-3 border-t border-gray-200 flex items-center space-x-2">
                        <select 
                          id={`select-${t.id}`}
                          className="text-sm border border-gray-300 rounded-lg px-2 py-1.5 flex-grow outline-none focus:border-emerald-500 focus:ring-1 focus:ring-emerald-500"
                        >
                          <option value="">Powiąż wpłatę z zamówieniem...</option>
                          {adminOrders.filter(o => o.status !== 'opłacone' && o.payment === 'blik').map(o => (
                            <option key={o.id} value={o.id}>{o.id} ({getUserName(o.userId)}) - {o.amount}zł</option>
                          ))}
                        </select>
                        <button 
                          onClick={() => {
                            const val = document.getElementById(`select-${t.id}`).value;
                            linkTransferToOrder(t.id, val);
                          }}
                          className="bg-gray-900 hover:bg-gray-800 text-white text-xs px-4 py-2 rounded-lg font-bold transition-colors"
                        >
                          Połącz
                        </button>
                      </div>
                    )}
                    {t.status === 'dopasowany' && (
                       <p className="text-xs text-emerald-700 mt-2 font-medium">Powiązano z rezerwacją: <span className="font-bold">{t.orderId}</span></p>
                    )}
                  </div>
                ))}
              </div>
            </div>
          </div>
        </div>
      );
    }

    // WIDOK 2: SZCZEGÓŁY UŻYTKOWNIKA (CRM)
    if (adminView === 'userDetail' && adminSelectedUserId) {
      const userToEdit = adminUsers.find(u => u.id === adminSelectedUserId);
      const userOrders = adminOrders.filter(o => o.userId === adminSelectedUserId);
      
      const handleSaveUser = (e) => {
        e.preventDefault();
        const formData = new FormData(e.target);
        const updatedUser = {
          ...userToEdit,
          name: formData.get('name'),
          email: formData.get('email'),
          phone: formData.get('phone'),
          address: formData.get('address')
        };
        setAdminUsers(adminUsers.map(u => u.id === adminSelectedUserId ? updatedUser : u));
        // Symulacja komunikatu o sukcesie
        const btn = document.getElementById('save-user-btn');
        const orgText = btn.innerHTML;
        btn.innerHTML = 'Zapisano!';
        btn.classList.add('bg-emerald-700');
        setTimeout(() => { btn.innerHTML = orgText; btn.classList.remove('bg-emerald-700'); }, 2000);
      };

      return (
        <div className="max-w-4xl mx-auto px-4 py-12">
          <button onClick={() => setAdminView('dashboard')} className="flex items-center text-gray-500 hover:text-emerald-600 mb-6 transition-colors">
            <ArrowLeft className="w-5 h-5 mr-2" /> Wróć do listy zamówień
          </button>
          
          <div className="flex items-center space-x-4 mb-8 bg-gray-50 p-6 rounded-3xl border border-gray-100">
            <div className="w-20 h-20 bg-emerald-100 rounded-full flex items-center justify-center text-emerald-600 shadow-sm">
              <User className="w-10 h-10" />
            </div>
            <div>
              <h2 className="text-3xl font-black text-gray-900">{userToEdit.name}</h2>
              <p className="text-gray-500 font-mono mt-1 bg-white px-2 py-1 rounded inline-block text-sm border border-gray-200">ID: {userToEdit.id}</p>
            </div>
          </div>

          <div className="bg-white rounded-2xl shadow-sm border border-gray-200 p-8 mb-8">
            <h3 className="font-bold text-xl mb-6 flex items-center pb-4 border-b border-gray-100"><Edit3 className="w-5 h-5 mr-2 text-emerald-600"/> Edycja danych do umów</h3>
            <form onSubmit={handleSaveUser}>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-6 mb-8">
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-2">Imię i nazwisko</label>
                  <input name="name" defaultValue={userToEdit.name} className="w-full p-3 border border-gray-300 rounded-xl focus:ring-2 focus:ring-emerald-500 outline-none transition-shadow" />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-2">Adres e-mail</label>
                  <input name="email" type="email" defaultValue={userToEdit.email} className="w-full p-3 border border-gray-300 rounded-xl focus:ring-2 focus:ring-emerald-500 outline-none transition-shadow" />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-2">Telefon kontaktowy</label>
                  <input name="phone" defaultValue={userToEdit.phone} className="w-full p-3 border border-gray-300 rounded-xl focus:ring-2 focus:ring-emerald-500 outline-none transition-shadow" />
                </div>
                <div className="md:col-span-2">
                  <label className="block text-sm font-medium text-gray-700 mb-2">Adres zamieszkania (pełny)</label>
                  <input name="address" defaultValue={userToEdit.address} className="w-full p-3 border border-gray-300 rounded-xl focus:ring-2 focus:ring-emerald-500 outline-none transition-shadow" />
                </div>
              </div>
              <div className="flex justify-end">
                <button id="save-user-btn" type="submit" className="bg-emerald-600 hover:bg-emerald-500 text-white font-bold py-3 px-8 rounded-xl flex items-center transition-all shadow-md">
                  <Save className="w-5 h-5 mr-2" /> Zapisz zmiany
                </button>
              </div>
            </form>
          </div>

          <div className="bg-white rounded-2xl shadow-sm border border-gray-200 p-8">
            <h3 className="font-bold text-xl mb-6 flex items-center pb-4 border-b border-gray-100"><ListOrdered className="w-5 h-5 mr-2 text-emerald-600"/> Historia rezerwacji klienta</h3>
            {userOrders.length === 0 ? (
              <p className="text-gray-500 italic bg-gray-50 p-4 rounded-xl text-center border border-gray-100">Brak historii zamówień dla tego klienta.</p>
            ) : (
              <div className="space-y-4">
                {userOrders.map(order => (
                   <div key={order.id} className="flex justify-between items-center border border-gray-100 bg-gray-50 p-5 rounded-xl hover:bg-white transition-colors">
                      <div>
                        <span className="font-black text-lg text-gray-900 block mb-1">{order.id}</span>
                        <span className="text-sm text-gray-600 font-medium">{order.items}</span>
                        <p className="text-xs text-gray-400 mt-1">Metoda: {order.payment}</p>
                      </div>
                      <div className="text-right">
                        <span className="font-black text-xl text-gray-900 block mb-2">{order.amount} zł</span>
                        <span className={`text-xs px-3 py-1.5 rounded-md font-bold ${order.status === 'opłacone' ? 'bg-emerald-100 text-emerald-700' : 'bg-yellow-100 text-yellow-700'}`}>
                          {order.status.toUpperCase()}
                        </span>
                      </div>
                   </div>
                ))}
              </div>
            )}
          </div>
        </div>
      );
    }
  };

  return (
    <div className="min-h-screen bg-white font-sans text-gray-900 flex flex-col">
      <Header />
      <AuthModal />
      
      <main className="flex-grow">
        {currentView === 'home' && (
          <>
            <div className="bg-emerald-900 text-white py-16 text-center">
              <h1 className="text-4xl md:text-5xl font-black mb-4">Wynajmij sprzęt na rodzinne wyprawy.</h1>
              <p className="text-emerald-200 text-lg">Radzymin i okolice. Ceny już od 30 zł / dobę.</p>
            </div>
            <section className="py-16 bg-gray-50">
              <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-8">
                {PRODUCTS.map(product => <ProductCard key={product.id} product={product} />)}
              </div>
            </section>
          </>
        )}

        {currentView === 'product' && selectedProduct && <ProductDetailView />}
        {currentView === 'checkout' && <CheckoutView />}
        {currentView === 'payment' && <PaymentView />}
        {currentView === 'success' && <SuccessView />}
        {currentView === 'user' && <UserPanelView />}
        {currentView === 'admin' && <AdminPanelView />}
      </main>

      <footer className="bg-gray-900 text-gray-400 py-12 border-t border-gray-800 text-sm">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 grid grid-cols-1 md:grid-cols-3 gap-8">
          <div>
            <span className="text-xl font-black tracking-tighter text-white mb-4 block">
              cargo.<span className="text-emerald-500">mleczki</span>.pl
            </span>
            <p>Prywatna wypożyczalnia sprzętu rowerowego.</p>
          </div>
          <div>
            <h4 className="text-white font-bold mb-4">Informacje Prawne</h4>
            <ul className="space-y-2">
              <li><a href="#" className="hover:text-emerald-400 transition-colors">Regulamin wypożyczalni</a></li>
              <li><a href="#" className="hover:text-emerald-400 transition-colors">Wzór Umowy Najmu</a></li>
              <li><a href="#" className="hover:text-emerald-400 transition-colors">Polityka Prywatności (RODO)</a></li>
            </ul>
          </div>
          <div>
            <h4 className="text-white font-bold mb-4">Pliki Cookies & Śledzenie</h4>
            <p className="flex items-start text-xs leading-relaxed"><ShieldCheck className="w-5 h-5 mr-2 text-emerald-500 flex-shrink-0" />
              Szanujemy Twoją prywatność. Strona wykorzystuje wyłącznie niezbędne pliki cookies sesyjne (np. do pamiętania koszyka i logowania). <strong>Nie stosujemy skryptów śledzących</strong> (Google Analytics, Pixel Facebooka itp.) i nie przekazujemy danych podmiotom trzecim.
            </p>
          </div>
        </div>
      </footer>
    </div>
  );
}