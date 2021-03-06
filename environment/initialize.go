package environment

import (
	"forza-garage/analytics"
	"forza-garage/authorization"
	"forza-garage/client"
	"forza-garage/database"
	"forza-garage/models"
	"os"

	influxdb2 "github.com/influxdata/influxdb-client-go"
	"go.mongodb.org/mongo-driver/mongo"
)

// upload serving endpoint's path is used
// also by the respective handlers to build response url
const UploadEndpoint = "/upload"

// Environment is used for dependency-injection (package de-coupling)
type Environment struct {
	Requests     *client.Registry
	Tracker      *analytics.Tracker
	Credentials  *authorization.Credentials
	UserModel    models.UserModel
	VoteModel    models.VoteModel
	CommentModel models.CommentModel
	UploadModel  models.UploadModel
	CourseModel  models.CourseModel
}

// newEnv operates as the constructor to initialize the collection references (private)
func newEnv(mongoClient *mongo.Client, influxClient *influxdb2.Client) *Environment {
	env := &Environment{}

	// ToDO: mongoClient für Modelle entfernen

	// ToDO: überlegen, ob zentral bei der connection als funktion getCollection
	mongoCollections := make(map[string]*mongo.Collection)
	mongoCollections["users"] = mongoClient.Database(os.Getenv("DB_NAME")).Collection("users") // ToDO: const
	mongoCollections["racing"] = mongoClient.Database(os.Getenv("DB_NAME")).Collection("racing")
	mongoCollections["social"] = mongoClient.Database(os.Getenv("DB_NAME")).Collection("social")

	// keep track of clients and their last requests
	env.Requests = new(client.Registry)
	env.Requests.Initialize()

	// prepare analytics gathering (profile visits)
	// always create the object so no futher checking is needed in the models
	env.Tracker = new(analytics.Tracker)
	env.Tracker.SetConnections(
		influxClient, // brauchts nicht mehr hier
		mongoCollections)
	// weil pointer umweg über variable
	fluxClient := *influxClient
	// ToDO: evtl. wäre eine Set-Funktion schöner
	env.Tracker.VisitorAPI.WriteAPI = fluxClient.WriteAPI(os.Getenv("ANALYTICS_ORG"), os.Getenv("ANALYTICS_VISITORS_BUCKET"))
	env.Tracker.VisitorAPI.QueryAPI = fluxClient.QueryAPI(os.Getenv("ANALYTICS_ORG"))
	env.Tracker.VisitorAPI.DeleteAPI = fluxClient.DeleteAPI()
	env.Tracker.SearchAPI.WriteAPI = fluxClient.WriteAPI(os.Getenv("ANALYTICS_ORG"), os.Getenv("ANALYTICS_SEARCHES_BUCKET"))
	env.Tracker.SearchAPI.QueryAPI = fluxClient.QueryAPI(os.Getenv("ANALYTICS_ORG"))
	// no deletes required for search bucket (TTL set)
	env.Tracker.Requests = env.Requests

	env.Credentials = new(authorization.Credentials)
	env.Credentials.SetConnections(mongoCollections)

	// upload muss vor user initialisiert werden
	env.UploadModel.Collection = mongoClient.Database(os.Getenv("DB_NAME")).Collection("uploads")
	//env.UploadModel.GetUserNameOID = env.UserModel.GetUserNameOID
	env.UploadModel.GetCredentials = env.Credentials.GetCredentials

	env.UserModel.Client = mongoClient
	env.UserModel.Collection = mongoClient.Database(os.Getenv("DB_NAME")).Collection("users") // ToDO: Const
	env.UserModel.Social = mongoClient.Database(os.Getenv("DB_NAME")).Collection("social")    // ToDO: Const
	env.UserModel.GetProfilePicture = env.UploadModel.GetMetaData

	env.UploadModel.GetUserNameOID = env.UserModel.GetUserNameOID // ToDo: Evtl. auch in author - REIHENFOLGE heikel

	// inject user model function to analytics tracker after its initialization
	env.Tracker.GetUserName = env.UserModel.GetUserName
	// env.Tracker.GetUserNameOID = env.UserModel.GetUserNameOID - nicht mehr benötigt; alte Lösung

	env.VoteModel.Collection = mongoClient.Database(os.Getenv("DB_NAME")).Collection("votes") // ToDO: Const
	env.VoteModel.GetUserNameOID = env.UserModel.GetUserNameOID

	env.CommentModel.Collection = mongoClient.Database(os.Getenv("DB_NAME")).Collection("comments")
	env.CommentModel.GetUserNameOID = env.UserModel.GetUserNameOID
	env.CommentModel.GetUserVotes = env.VoteModel.GetUserVotes

	env.CourseModel.Client = mongoClient
	env.CourseModel.Collection = mongoClient.Database(os.Getenv("DB_NAME")).Collection("racing") // ToDO: Const
	// Funktionen aus dem User Model in's Course model "injecten"
	env.CourseModel.GetUserName = env.UserModel.GetUserName
	env.CourseModel.CredentialsReader = env.UserModel.GetCredentials // ToDo: auf authorization umstellen
	env.CourseModel.GetUserVote = env.VoteModel.GetUserVote
	// inject analytics
	// env.CourseModel.Tracker = env.Tracker

	return env
}

// Env is the singleton registry
var Env *Environment

// InitializeModels injects the database connection to the models
// (do not confuse with package init)
func InitializeModels() {
	/*env = &Env{
	userModel: models.UserModel{Client: database.GetConnection()}}*/

	Env = newEnv(database.GetConnection(), database.GetInfluxConnection())
}
