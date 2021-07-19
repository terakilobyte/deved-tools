package helpers

import "os"

// mongodb vars
var DB = "tags"
var DBURI = os.Getenv("TAG_MONITOR")

type MongoTags struct {
	Repo string   `bson:"repo"`
	Tags []string `bson:"tags"`
}
