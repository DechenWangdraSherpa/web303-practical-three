#!/bin/bash
set -e

echo "🚀 Building microservices development environment..."

# Function to check if required tools are installed
check_dependencies() {
    echo "📋 Checking dependencies..."

    command -v docker >/dev/null 2>&1 || { echo "❌ Docker is required but not installed."; exit 1; }
    command -v docker-compose >/dev/null 2>&1 || { echo "❌ Docker Compose is required but not installed."; exit 1; }
    command -v protoc >/dev/null 2>&1 || { echo "❌ protoc is required but not installed."; exit 1; }

    echo "✅ All dependencies found"
}

# Generate proto files
generate_proto_files() {
    echo "🔧 Generating proto files..."

    # Clean previous generations
    rm -rf proto/gen
    mkdir -p proto/gen

    # Generate Go code
    protoc --go_out=./proto/gen --go_opt=paths=source_relative \
           --go-grpc_out=./proto/gen --go-grpc_opt=paths=source_relative \
           proto/*.proto

    echo "✅ Proto files generated"
}

# Copy proto files to each service for Docker build context
distribute_proto_files() {
    echo "📦 Distributing proto files to services..."

    services=("api-gateway" "services/users-service" "services/products-service")

    for service in "${services[@]}"; do
        echo "  📂 Copying to $service..."
        
        # Check if service directory exists
        if [ ! -d "$service" ]; then
            echo "❌ Service directory $service does not exist!"
            exit 1
        fi

        # Create proto directory
        mkdir -p "$service/proto" || {
            echo "❌ Failed to create proto directory for $service"
            exit 1
        }

        # Copy proto files with error checking
        if ! cp -r proto/* "$service/proto/" 2>/dev/null; then
            echo "❌ Failed to copy proto files to $service"
            exit 1
        fi

        echo "  ✅ Successfully copied proto files to $service"
    done

    echo "✅ Proto files distributed to all services"
}

# Clean up old containers and images
cleanup() {
    echo "🧹 Cleaning up old containers..."
    docker-compose down --remove-orphans 2>/dev/null || true
    docker system prune -f --volumes 2>/dev/null || true
}

# Build and start services
build_and_start() {
    echo "🏗️  Building and starting services..."

    # Build with no cache to ensure fresh build
    docker-compose build --no-cache

    # Start services
    docker-compose up -d

    # Wait for services to be ready
    echo "⏳ Waiting for services to be ready..."
    sleep 30

    # Check service health
    check_service_health
}

# Check if services are responding
check_service_health() {
    echo "🔍 Checking service health..."

    # Check Consul
    if curl -s http://localhost:8500/v1/status/leader >/dev/null; then
        echo "✅ Consul is healthy"
    else
        echo "❌ Consul is not responding"
    fi

    # Check API Gateway
    if curl -s http://localhost:8080/health >/dev/null 2>&1; then
        echo "✅ API Gateway is healthy"
    else
        echo "⚠️  API Gateway may still be starting..."
    fi

    echo "🎉 Build complete! Services are available at:"
    echo "   - Consul UI: http://localhost:8500"
    echo "   - API Gateway: http://localhost:8080"
    echo "   - Users DB: localhost:5432"
    echo "   - Products DB: localhost:5433"
}

# Main execution
main() {
    check_dependencies
    generate_proto_files
    distribute_proto_files
    cleanup
    build_and_start
}

# Run main function
main "$@"