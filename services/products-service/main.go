package main

import (
    "context"
    "fmt"
    "log"
    "net"
    "os"
    "time"

    "google.golang.org/grpc"
    "google.golang.org/grpc/health"
    "google.golang.org/grpc/health/grpc_health_v1"
    "gorm.io/driver/postgres"
    "gorm.io/gorm"

    pb "products-service/proto/gen/proto"
    consulapi "github.com/hashicorp/consul/api"
)

const serviceName = "products-service"
const servicePort = 50052

type Product struct {
    gorm.Model
    Name  string
    Price float64
}

type server struct {
    pb.UnimplementedProductServiceServer
    db *gorm.DB
}

func (s *server) CreateProduct(ctx context.Context, req *pb.CreateProductRequest) (*pb.ProductResponse, error) {
    product := Product{Name: req.Name, Price: req.Price}
    if result := s.db.Create(&product); result.Error != nil {
        return nil, result.Error
    }
    return &pb.ProductResponse{Product: &pb.Product{Id: fmt.Sprint(product.ID), Name: product.Name, Price: product.Price}}, nil
}

func (s *server) GetProduct(ctx context.Context, req *pb.GetProductRequest) (*pb.ProductResponse, error) {
    var product Product
    if result := s.db.First(&product, req.Id); result.Error != nil {
        return nil, result.Error
    }
    return &pb.ProductResponse{Product: &pb.Product{Id: fmt.Sprint(product.ID), Name: product.Name, Price: product.Price}}, nil
}

func main() {
    // Wait for database to be ready
    time.Sleep(10 * time.Second)

    // Connect to database with retry logic
    db := connectToDatabaseWithRetry()
    db.AutoMigrate(&Product{})

    // Start gRPC server
    lis, err := net.Listen("tcp", fmt.Sprintf(":%d", servicePort))
    if err != nil {
        log.Fatalf("Failed to listen: %v", err)
    }
    s := grpc.NewServer()
    pb.RegisterProductServiceServer(s, &server{db: db})

    // Register health check
    healthServer := health.NewServer()
    grpc_health_v1.RegisterHealthServer(s, healthServer)
    healthServer.SetServingStatus("products.ProductService", grpc_health_v1.HealthCheckResponse_SERVING)

    // Register with Consul
    if err := registerServiceWithConsul(); err != nil {
        log.Fatalf("Failed to register with Consul: %v", err)
    }

    log.Printf("%s gRPC server listening at %v", serviceName, lis.Addr())
    if err := s.Serve(lis); err != nil {
        log.Fatalf("Failed to serve: %v", err)
    }
}

func connectToDatabaseWithRetry() *gorm.DB {
    dsn := "host=products-db user=user password=password dbname=products_db port=5432 sslmode=disable"

    var db *gorm.DB
    var err error

    for i := 0; i < 30; i++ {
        db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
        if err == nil {
            log.Println("Successfully connected to database")
            break
        }

        log.Printf("Failed to connect to database (attempt %d/30): %v", i+1, err)
        time.Sleep(10 * time.Second)
    }

    if err != nil {
        log.Fatalf("Could not connect to database after 30 attempts: %v", err)
    }

    return db
}

func registerServiceWithConsul() error {
    config := consulapi.DefaultConfig()
    if addr := os.Getenv("CONSUL_HTTP_ADDR"); addr != "" {
        config.Address = addr
    }

    consul, err := consulapi.NewClient(config)
    if err != nil {
        return err
    }

    // Use the service name as the address within the Docker network
    registration := &consulapi.AgentServiceRegistration{
        ID:      serviceName,
        Name:    serviceName,
        Port:    servicePort,
        Address: serviceName,
        Check: &consulapi.AgentServiceCheck{
            GRPC:                           fmt.Sprintf("%s:%d", serviceName, servicePort),
            Interval:                       "10s",
            DeregisterCriticalServiceAfter: "30s",
        },
    }

    err = consul.Agent().ServiceRegister(registration)
    if err == nil {
        log.Printf("Successfully registered %s with Consul at %s:%d", serviceName, serviceName, servicePort)
    }
    return err
}