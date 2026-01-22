-- Eighty Twenty Ops - Initial Schema

-- Users table
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('admin', 'moderator', 'community_officer')),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Leads table
CREATE TABLE IF NOT EXISTS leads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    full_name TEXT NOT NULL,
    phone TEXT UNIQUE NOT NULL,
    source TEXT,
    notes TEXT,
    status TEXT NOT NULL DEFAULT 'lead_created' CHECK (status IN (
        'lead_created', 'test_booked', 'tested', 'offer_sent', 'booking_confirmed',
        'paid_full', 'deposit_paid', 'waiting_for_round', 'schedule_assigned', 'ready_to_start'
    )),
    created_by_user_id UUID REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Placement tests table
CREATE TABLE IF NOT EXISTS placement_tests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID UNIQUE REFERENCES leads(id) ON DELETE CASCADE,
    test_date DATE,
    test_time TIME,
    test_type TEXT CHECK (test_type IN ('online', 'live')),
    assigned_level INTEGER CHECK (assigned_level >= 1 AND assigned_level <= 4),
    test_notes TEXT,
    run_by_user_id UUID REFERENCES users(id),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Offers table
CREATE TABLE IF NOT EXISTS offers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID UNIQUE REFERENCES leads(id) ON DELETE CASCADE,
    bundle_levels INTEGER CHECK (bundle_levels >= 1 AND bundle_levels <= 4),
    base_price INTEGER,
    discount_value INTEGER,
    discount_type TEXT CHECK (discount_type IN ('amount', 'percent')),
    final_price INTEGER,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Bookings table
CREATE TABLE IF NOT EXISTS bookings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID UNIQUE REFERENCES leads(id) ON DELETE CASCADE,
    book_format TEXT CHECK (book_format IN ('pdf', 'printed')),
    address TEXT,
    city TEXT,
    delivery_notes TEXT,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Payments table
CREATE TABLE IF NOT EXISTS payments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID UNIQUE REFERENCES leads(id) ON DELETE CASCADE,
    payment_type TEXT CHECK (payment_type IN ('full', 'deposit')),
    amount_paid INTEGER DEFAULT 0,
    remaining_balance INTEGER DEFAULT 0,
    payment_date DATE,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Scheduling table
CREATE TABLE IF NOT EXISTS scheduling (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID UNIQUE REFERENCES leads(id) ON DELETE CASCADE,
    expected_round TEXT,
    class_days TEXT,
    class_time TIME,
    start_date DATE,
    start_time TIME,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Shipping table
CREATE TABLE IF NOT EXISTS shipping (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID UNIQUE REFERENCES leads(id) ON DELETE CASCADE,
    shipment_status TEXT CHECK (shipment_status IN ('pending', 'sent', 'delivered')),
    shipment_date DATE,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_leads_status ON leads(status);
CREATE INDEX IF NOT EXISTS idx_leads_phone ON leads(phone);
CREATE INDEX IF NOT EXISTS idx_leads_created_at ON leads(created_at);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
