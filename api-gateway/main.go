package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "api-gateway/proto/gen/proto"
	consulapi "github.com/hashicorp/consul/api"
)

type ServiceDiscovery struct {
	consul      *consulapi.Client
	mu          sync.RWMutex
	connections map[string]*grpc.ClientConn
}

type UserPurchaseData struct {
	User    *pb.User    `json:"user"`
	Product *pb.Product `json:"product"`
}

var sd *ServiceDiscovery

func main() {
	// Initialize service discovery
	config := consulapi.DefaultConfig()
	if addr := os.Getenv("CONSUL_HTTP_ADDR"); addr != "" {
		config.Address = addr
	}

	consul, err := consulapi.NewClient(config)
	if err != nil {
		log.Fatalf("Failed to create consul client: %v", err)
	}

	sd = &ServiceDiscovery{
		consul:      consul,
		connections: make(map[string]*grpc.ClientConn),
	}

	// Wait for services to be ready
	log.Println("Waiting for services to register with Consul...")
	time.Sleep(15 * time.Second)

	r := mux.NewRouter()

	// User routes
	r.HandleFunc("/api/users", createUserHandler).Methods("POST")
	r.HandleFunc("/api/users/{id}", getUserHandler).Methods("GET")

	// Product routes
	r.HandleFunc("/api/products", createProductHandler).Methods("POST")
	r.HandleFunc("/api/products/{id}", getProductHandler).Methods("GET")

	// Composite endpoint
	r.HandleFunc("/api/purchases/user/{userId}/product/{productId}", getPurchaseDataHandler).Methods("GET")

	// Health check endpoint
	r.HandleFunc("/health", healthHandler).Methods("GET")

	log.Println("API Gateway listening on port 8080...")
	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatalf("Failed to start API Gateway: %v", err)
	}
}

func (sd *ServiceDiscovery) getServiceConnection(serviceName string) (*grpc.ClientConn, error) {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	// Check if we already have a connection
	if conn, exists := sd.connections[serviceName]; exists {
		return conn, nil
	}

	// Discover service
	services, _, err := sd.consul.Health().Service(serviceName, "", true, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to discover service %s: %w", serviceName, err)
	}

	if len(services) == 0 {
		return nil, fmt.Errorf("no healthy instances of service %s found", serviceName)
	}

	// Use first healthy instance
	service := services[0].Service
	address := fmt.Sprintf("%s:%d", service.Address, service.Port)

	// Create gRPC connection
	conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to service %s at %s: %w", serviceName, address, err)
	}

	sd.connections[serviceName] = conn
	log.Printf("Connected to %s at %s", serviceName, address)
	return conn, nil
}

func getUsersClient() (pb.UserServiceClient, error) {
	conn, err := sd.getServiceConnection("users-service")
	if err != nil {
		return nil, err
	}
	return pb.NewUserServiceClient(conn), nil
}

func getProductsClient() (pb.ProductServiceClient, error) {
	conn, err := sd.getServiceConnection("products-service")
	if err != nil {
		return nil, err
	}
	return pb.NewProductServiceClient(conn), nil
}

// Health check handler
func healthHandler(w http.ResponseWriter, r *http.Request) {
	response := map[string]string{
		"status":    "healthy",
		"timestamp": time.Now().Format(time.RFC3339),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// User Handlers
func createUserHandler(w http.ResponseWriter, r *http.Request) {
	client, err := getUsersClient()
	if err != nil {
		http.Error(w, fmt.Sprintf("Service unavailable: %v", err), http.StatusServiceUnavailable)
		return
	}

	var req pb.CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	res, err := client.CreateUser(context.Background(), &req)
	if err != nil {
		log.Printf("Error creating user: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(res.User)
}

func getUserHandler(w http.ResponseWriter, r *http.Request) {
	client, err := getUsersClient()
	if err != nil {
		http.Error(w, fmt.Sprintf("Service unavailable: %v", err), http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(r)
	id := vars["id"]

	res, err := client.GetUser(context.Background(), &pb.GetUserRequest{Id: id})
	if err != nil {
		log.Printf("Error getting user: %v", err)
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res.User)
}

// Product Handlers
func createProductHandler(w http.ResponseWriter, r *http.Request) {
	client, err := getProductsClient()
	if err != nil {
		http.Error(w, fmt.Sprintf("Service unavailable: %v", err), http.StatusServiceUnavailable)
		return
	}

	var req pb.CreateProductRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	res, err := client.CreateProduct(context.Background(), &req)
	if err != nil {
		log.Printf("Error creating product: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(res.Product)
}

func getProductHandler(w http.ResponseWriter, r *http.Request) {
	client, err := getProductsClient()
	if err != nil {
		http.Error(w, fmt.Sprintf("Service unavailable: %v", err), http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(r)
	id := vars["id"]

	res, err := client.GetProduct(context.Background(), &pb.GetProductRequest{Id: id})
	if err != nil {
		log.Printf("Error getting product: %v", err)
		http.Error(w, "Product not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res.Product)
}

// Fixed composite endpoint with proper service discovery
func getPurchaseDataHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userId := vars["userId"]
	productId := vars["productId"]

	var wg sync.WaitGroup
	var user *pb.User
	var product *pb.Product
	var userErr, productErr error

	wg.Add(2)

	// Fetch user data
	go func() {
		defer wg.Done()
		client, err := getUsersClient()
		if err != nil {
			userErr = err
			return
		}
		res, err := client.GetUser(context.Background(), &pb.GetUserRequest{Id: userId})
		if err != nil {
			userErr = err
			return
		}
		user = res.User
	}()

	// Fetch product data
	go func() {
		defer wg.Done()
		client, err := getProductsClient()
		if err != nil {
			productErr = err
			return
		}
		res, err := client.GetProduct(context.Background(), &pb.GetProductRequest{Id: productId})
		if err != nil {
			productErr = err
			return
		}
		product = res.Product
	}()

	wg.Wait()

	if userErr != nil {
		log.Printf("Error getting user: %v", userErr)
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	if productErr != nil {
		log.Printf("Error getting product: %v", productErr)
		http.Error(w, "Product not found", http.StatusNotFound)
		return
	}

	purchaseData := UserPurchaseData{
		User:    user,
		Product: product,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(purchaseData)
}