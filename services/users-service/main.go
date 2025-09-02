// services/users-service/main.go
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

const serviceName = "users-service"
const servicePort = 50051

// GORM model for our User
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
    // 1. Connect to the database
    dsn := "host=users-db user=user password=password dbname=users_db port=5432 sslmode=disable"
    db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
    if err != nil {
        log.Fatalf("Failed to connect to database: %v", err)
    }
    db.AutoMigrate(&User{})

    // 2. Start the gRPC server
    lis, err := net.Listen("tcp", fmt.Sprintf(":%d", servicePort))
    if err != nil {
        log.Fatalf("Failed to listen: %v", err)
    }
    s := grpc.NewServer()
    pb.RegisterUserServiceServer(s, &server{db: db})

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
