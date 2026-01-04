package main

import (
	"context"
	"fmt"
	"log"
	"os"

	firebase "firebase.google.com/go/v4"
	"github.com/cloudinary/cloudinary-go/v2"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/api/option"
)

func main() {
	// Load .env
	if err := godotenv.Load(); err != nil {
		log.Fatal("Error loading .env file")
	}

	// Test MongoDB
	fmt.Println("Testing MongoDB connection...")
	mongoURI := os.Getenv("MONGODB_URI")
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatal("MongoDB connection failed:", err)
	}
	defer client.Disconnect(context.Background())
	
	if err := client.Ping(context.Background(), nil); err != nil {
		log.Fatal("MongoDB ping failed:", err)
	}
	fmt.Println("✅ MongoDB connected successfully!")

	// Test Firebase (Auth only)
	fmt.Println("\nTesting Firebase Auth connection...")
	firebasePath := os.Getenv("FIREBASE_SERVICE_ACCOUNT_PATH")
	opt := option.WithCredentialsFile(firebasePath)
	
	app, err := firebase.NewApp(context.Background(), nil, opt)
	if err != nil {
		log.Fatal("Firebase initialization failed:", err)
	}
	
	_, err = app.Auth(context.Background())
	if err != nil {
		log.Fatal("Firebase Auth client failed:", err)
	}
	fmt.Println("✅ Firebase Auth connected successfully!")

	// Test Cloudinary
	fmt.Println("\nTesting Cloudinary connection...")
	cloudName := os.Getenv("CLOUDINARY_CLOUD_NAME")
	apiKey := os.Getenv("CLOUDINARY_API_KEY")
	apiSecret := os.Getenv("CLOUDINARY_API_SECRET")

	if cloudName == "" || apiKey == "" || apiSecret == "" {
		log.Fatal("Cloudinary credentials missing in .env")
	}

	cldURL := fmt.Sprintf("cloudinary://%s:%s@%s", apiKey, apiSecret, cloudName)
	cld, err := cloudinary.NewFromURL(cldURL)
	if err != nil {
		log.Fatal("Cloudinary initialization failed:", err)
	}

	if cld.Config.Cloud.CloudName != cloudName {
		log.Fatal("Cloudinary config mismatch")
	}
	fmt.Println("✅ Cloudinary connected successfully!")

	fmt.Println("\n🎉 All systems ready! You can start implementing auth.")
	fmt.Println("\nCloudinary Details:")
	fmt.Printf("  Cloud Name: %s\n", cloudName)
	fmt.Printf("  Upload Folder: %s\n", os.Getenv("CLOUDINARY_UPLOAD_FOLDER"))
}
