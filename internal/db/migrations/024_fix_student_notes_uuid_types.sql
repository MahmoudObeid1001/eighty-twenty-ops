-- Fix student_notes table: ensure lead_id and created_by_user_id are UUID (not TEXT)
-- Root cause: Table was created with TEXT types but queries pass UUID parameters
-- This causes "operator does not exist: text = uuid" errors

-- Fix lead_id: convert TEXT to UUID if needed
DO $$
BEGIN
    -- Check current type
    IF EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'student_notes' 
        AND column_name = 'lead_id' 
        AND data_type = 'text'
    ) THEN
        -- Drop foreign key constraint if it exists (will be re-added)
        ALTER TABLE student_notes DROP CONSTRAINT IF EXISTS student_notes_lead_id_fkey;
        
        -- Convert TEXT to UUID (handles existing valid UUID strings)
        ALTER TABLE student_notes
        ALTER COLUMN lead_id TYPE uuid USING lead_id::uuid;
        
        -- Re-add foreign key constraint
        ALTER TABLE student_notes
        ADD CONSTRAINT student_notes_lead_id_fkey 
        FOREIGN KEY (lead_id) REFERENCES leads(id) ON DELETE CASCADE;
        
        -- Ensure NOT NULL
        ALTER TABLE student_notes ALTER COLUMN lead_id SET NOT NULL;
    ELSIF EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'student_notes' 
        AND column_name = 'lead_id' 
        AND data_type = 'uuid'
    ) THEN
        -- Already UUID, just ensure constraints
        ALTER TABLE student_notes ALTER COLUMN lead_id SET NOT NULL;
        IF NOT EXISTS (
            SELECT 1 FROM pg_constraint 
            WHERE conname = 'student_notes_lead_id_fkey'
        ) THEN
            ALTER TABLE student_notes
            ADD CONSTRAINT student_notes_lead_id_fkey 
            FOREIGN KEY (lead_id) REFERENCES leads(id) ON DELETE CASCADE;
        END IF;
    END IF;
END $$;

-- Fix created_by_user_id: convert TEXT to UUID if needed
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'student_notes' 
        AND column_name = 'created_by_user_id' 
        AND data_type = 'text'
    ) THEN
        -- Drop foreign key constraint if it exists
        ALTER TABLE student_notes DROP CONSTRAINT IF EXISTS student_notes_created_by_user_id_fkey;
        
        -- Convert TEXT to UUID (handle NULL and empty strings)
        ALTER TABLE student_notes
        ALTER COLUMN created_by_user_id TYPE uuid 
        USING CASE 
            WHEN created_by_user_id IS NULL OR created_by_user_id = '' OR created_by_user_id = 'null' THEN NULL
            ELSE created_by_user_id::uuid
        END;
        
        -- Re-add foreign key constraint
        ALTER TABLE student_notes
        ADD CONSTRAINT student_notes_created_by_user_id_fkey 
        FOREIGN KEY (created_by_user_id) REFERENCES users(id);
    ELSIF EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'student_notes' 
        AND column_name = 'created_by_user_id' 
        AND data_type = 'uuid'
    ) THEN
        -- Already UUID, just ensure constraint
        IF NOT EXISTS (
            SELECT 1 FROM pg_constraint 
            WHERE conname = 'student_notes_created_by_user_id_fkey'
        ) THEN
            ALTER TABLE student_notes
            ADD CONSTRAINT student_notes_created_by_user_id_fkey 
            FOREIGN KEY (created_by_user_id) REFERENCES users(id);
        END IF;
    END IF;
END $$;
