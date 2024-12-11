package database

import (
	"context"
	"fmt"
	"testing"
)

func TestNormalizationStep_Process(t *testing.T) {
	mongo,_ := NewMongoDB(context.Background())

	fmt.Println(mongo.GetListings())
}
