CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS files (
    file_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    file_name VARCHAR(180) NOT NULL,
    file_ext VARCHAR(80) NOT NULL,
    file_mimetype VARCHAR(100) NOT NULL,
    uploaded_at TIMESTAMPTZ DEFAULT NOW() NOT NULL
);
