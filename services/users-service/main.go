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

    pb "users-service/proto/gen/proto"
    consulapi "github.com/hashicorp/consul/api"
)

const serviceName = "users-service"
const servicePort = 50051

type User struct {
    gorm.Model
    Name  string
    Email string `gorm:"unique"`
}

type server struct {
    pb.UnimplementedUserServiceServer
    db *gorm.DB
}

func (s *server) CreateUser(ctx context.Context, req *pb.CreateUserRequest) (*pb.UserResponse, error) {
    user := User{Name: req.Name, Email: req.Email}
    if result := s.db.Create(&user); result.Error != nil {
        return nil, result.Error
    }
    return &pb.UserResponse{User: &pb.User{Id: fmt.Sprint(user.ID), Name: user.Name, Email: user.Email}}, nil
}

func (s *server) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.UserResponse, error) {
    var user User
    if result := s.db.First(&user, req.Id); result.Error != nil {
        return nil, result.Error
    }
    return &pb.UserResponse{User: &pb.User{Id: fmt.Sprint(user.ID), Name: user.Name, Email: user.Email}}, nil
}

func main() {
    // Wait for database to be ready
    time.Sleep(10 * time.Second)

    // Connect to database with retry logic
    db := connectToDatabaseWithRetry()
    db.AutoMigrate(&User{})

    // Start gRPC server
    lis, err := net.Listen("tcp", fmt.Sprintf(":%d", servicePort))
    if err != nil {
        log.Fatalf("Failed to listen: %v", err)
    }
    s := grpc.NewServer()
    pb.RegisterUserServiceServer(s, &server{db: db})

    // Register health check
    healthServer := health.NewServer()
    grpc_health_v1.RegisterHealthServer(s, healthServer)
    healthServer.SetServingStatus("users.UserService", grpc_health_v1.HealthCheckResponse_SERVING)

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
    dsn := "host=users-db user=user password=password dbname=users_db port=5432 sslmode=disable"

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