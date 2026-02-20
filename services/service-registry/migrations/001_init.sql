-- services/service-registry/migrations/001_init.sql
-- +goose Up
-- +goose StatementBegin

-- Create enum for service status
CREATE TYPE service_status AS ENUM ('active', 'inactive', 'degraded', 'down');

-- Services table
CREATE TABLE IF NOT EXISTS services (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL UNIQUE,
    version VARCHAR(50) NOT NULL,
    spec JSONB NOT NULL,
    status service_status NOT NULL DEFAULT 'active',
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    last_heartbeat TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Create indexes
CREATE INDEX idx_services_name ON services(name);
CREATE INDEX idx_services_status ON services(status);
CREATE INDEX idx_services_last_heartbeat ON services(last_heartbeat);

-- Endpoints table for service instances
CREATE TABLE IF NOT EXISTS endpoints (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    url VARCHAR(255) NOT NULL,
    protocol VARCHAR(10) NOT NULL, -- http, grpc, ws
    methods TEXT[],
    weight INT DEFAULT 1,
    healthy BOOLEAN DEFAULT TRUE,
    last_check TIMESTAMP WITH TIME ZONE,
    response_time BIGINT, -- in milliseconds
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(service_id, url)
);

CREATE INDEX idx_endpoints_service_id ON endpoints(service_id);
CREATE INDEX idx_endpoints_healthy ON endpoints(healthy);

-- Service dependencies table
CREATE TABLE IF NOT EXISTS dependencies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    depends_on UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(service_id, depends_on)
);

-- Service versions history
CREATE TABLE IF NOT EXISTS service_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    version VARCHAR(50) NOT NULL,
    spec JSONB NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_service_versions_service_id ON service_versions(service_id);

-- Create function to update updated_at
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Create triggers
CREATE TRIGGER update_services_updated_at BEFORE UPDATE
    ON services FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_endpoints_updated_at BEFORE UPDATE
    ON endpoints FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Insert some initial data if needed
-- INSERT INTO services (name, version, spec) VALUES 
--     ('service-registry', '1.0.0', '{"openapi":"3.0.0"}'::jsonb);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS service_versions;
DROP TABLE IF EXISTS dependencies;
DROP TABLE IF EXISTS endpoints;
DROP TABLE IF EXISTS services;
DROP TYPE IF EXISTS service_status;
-- +goose StatementEnd