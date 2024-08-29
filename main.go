package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

type App struct {
	s3Client *s3.S3
	bucket   string
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("Application error: %v", err)
	}
}

func run() error {
	app, err := newApp()
	if err != nil {
		return fmt.Errorf("failed to initialize app: %w", err)
	}

	r := setupRouter(app)
	return r.Run(":8080")
}

func newApp() (*App, error) {
	if err := godotenv.Load(); err != nil {
		return nil, fmt.Errorf("error loading .env file: %w", err)
	}

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(os.Getenv("AWS_REGION")),
		Credentials: credentials.NewStaticCredentials(
			os.Getenv("AWS_ACCESS_KEY_ID"),
			os.Getenv("AWS_SECRET_ACCESS_KEY"),
			"",
		),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %w", err)
	}

	return &App{
		s3Client: s3.New(sess),
		bucket:   os.Getenv("S3_BUCKET_NAME"),
	}, nil
}

func setupRouter(app *App) *gin.Engine {
	r := gin.Default()

	config := cors.DefaultConfig()
	config.AllowAllOrigins = true
	r.Use(cors.New(config))

	r.POST("/upload", app.uploadHandler)
	r.GET("/download/:key", app.downloadHandler)
	r.GET("/list", app.listHandler)

	return r
}

func (app *App) uploadHandler(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	defer file.Close()

	_, err = app.s3Client.PutObject(&s3.PutObjectInput{
		Bucket: aws.String(app.bucket),
		Key:    aws.String(header.Filename),
		Body:   file,
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "File uploaded successfully"})
}

func (app *App) downloadHandler(c *gin.Context) {
	key := c.Param("key")

	result, err := app.s3Client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(app.bucket),
		Key:    aws.String(key),
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer result.Body.Close()

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", key))
	c.Header("Content-Type", aws.StringValue(result.ContentType))

	if _, err = io.Copy(c.Writer, result.Body); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

func (app *App) listHandler(c *gin.Context) {
	resp, err := app.s3Client.ListObjectsV2(&s3.ListObjectsV2Input{
		Bucket: aws.String(app.bucket),
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	objects := make([]string, len(resp.Contents))
	for i, item := range resp.Contents {
		objects[i] = aws.StringValue(item.Key)
	}

	c.JSON(http.StatusOK, gin.H{"objects": objects})
}
