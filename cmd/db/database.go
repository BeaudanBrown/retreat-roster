package db

import (
	"context"

	"go.mongodb.org/mongo-driver/mongo"
)


type Database struct {
  DB *mongo.Database
  Context context.Context
}

