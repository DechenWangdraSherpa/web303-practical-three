// services/products-service/main.go
package main

import (
    "context"
    "fmt"
    "log"
    "net"
    "os"

    "google.golang.org/grpc"
    "gorm.io/driver/postgres"
    "gorm.io/gorm"

    pb "practical-three/proto/gen"
    consulapi "github.com/hashicorp/consul/api"
)

const serviceName = "products-service"
const servicePort = 50052

// GORM model for our Product
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
    // 1. Connect to the database
    dsn := "host=products-db user=user password=password dbname=products_db port=5432 sslmode=disable"
    db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
    if err != nil {
        log.Fatalf("Failed to connect to database: %v", err)
    }
    db.AutoMigrate(&Product{})

    // 2. Start the gRPC server
    lis, err := net.Listen("tcp", fmt.Sprintf(":%d", servicePort))
    if err != nil {
        log.Fatalf("Failed to listen: %v", err)
    }
    s := grpc.NewServer()
    pb.RegisterProductServiceServer(s, &server{db: db})

    // 3. Register with Consul
    if err := registerServiceWithConsul(); err != nil {
        log.Fatalf("Failed to register with Consul: %v", err)
    }

    log.Printf("%s gRPC server listening at %v", serviceName, lis.Addr())
    if err := s.Serve(lis); err != nil {
        log.Fatalf("Failed to serve: %v", err)
    }
}

func registerServiceWithConsul() error {
    config := consulapi.DefaultConfig()
    consul, err := consulapi.NewClient(config)
    if err != nil {
        return err
    }

    hostname, err := os.Hostname()
    if err != nil {
        return err
    }

    registration := &consulapi.AgentServiceRegistration{
        ID:      fmt.Sprintf("%s-%s", serviceName, hostname),
        Name:    serviceName,
        Port:    servicePort,
        Address: hostname,
    }

    return consul.Agent().ServiceRegister(registration)
}