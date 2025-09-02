// api-gateway/main.go
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/mux"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "practical-three/proto/gen"
)

var usersClient pb.UserServiceClient
var productsClient pb.ProductServiceClient

// A struct to hold the aggregated data
type UserPurchaseData struct {
	User    *pb.User    `json:"user"`
	Product *pb.Product `json:"product"`
}

func main() {
	// Connect to the users-service
	userConn, err := grpc.Dial("users-service:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Did not connect to users-service: %v", err)
	}
	defer userConn.Close()
	usersClient = pb.NewUserServiceClient(userConn)

	// Connect to the products-service
	productConn, err := grpc.Dial("products-service:50052", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Did not connect to products-service: %v", err)
	}
	defer productConn.Close()
	productsClient = pb.NewProductServiceClient(productConn)

	r := mux.NewRouter()
	// User routes
	r.HandleFunc("/api/users", createUserHandler).Methods("POST")
	r.HandleFunc("/api/users/{id}", getUserHandler).Methods("GET")
	// Product routes
	r.HandleFunc("/api/products", createProductHandler).Methods("POST")
	r.HandleFunc("/api/products/{id}", getProductHandler).Methods("GET")

	// The new endpoint to get combined data
	r.HandleFunc("/api/purchases/user/{userId}/product/{productId}", getPurchaseDataHandler).Methods("GET")

	log.Println("API Gateway listening on port 8080...")
	http.ListenAndServe(":8080", r)
}

// User Handlers
func createUserHandler(w http.ResponseWriter, r *http.Request) {
	var req pb.CreateUserRequest
	json.NewDecoder(r.Body).Decode(&req)
	res, err := usersClient.CreateUser(context.Background(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res.User)
}

func getUserHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	res, err := usersClient.GetUser(context.Background(), &pb.GetUserRequest{Id: id})
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res.User)
}

// Product Handlers
func createProductHandler(w http.ResponseWriter, r *http.Request) {
	var req pb.CreateProductRequest
	json.NewDecoder(r.Body).Decode(&req)
	res, err := productsClient.CreateProduct(context.Background(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res.Product)
}

func getProductHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	res, err := productsClient.GetProduct(context.Background(), &pb.GetProductRequest{Id: id})
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res.Product)
}

// New handler for combined data
func getPurchaseDataHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userId := vars["userId"]
	productId := vars["productId"]

	var wg sync.WaitGroup
	var user *pb.User
	var product *pb.Product
	var userErr, productErr error

	wg.Add(2)

	go func() {
		defer wg.Done()
		res, err := usersClient.GetUser(context.Background(), &pb.GetUserRequest{Id: userId})
		if err != nil {
			userErr = err
			return
		}
		user = res.User
	}()

	go func() {
		defer wg.Done()
		res, err := productsClient.GetProduct(context.Background(), &pb.GetProductRequest{Id: productId})
		if err != nil {
			productErr = err
			return
		}
		product = res.Product
	}()

	wg.Wait()

	if userErr != nil || productErr != nil {
		http.Error(w, "Could not retrieve all data", http.StatusNotFound)
		return
	}

	purchaseData := UserPurchaseData{
		User:    user,
		Product: product,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(purchaseData)
}