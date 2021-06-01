package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// File Name Convention at Destination:
// otype_uuid.ext

// UploadInfo contains the meta data of an uploaded file
type UploadInfo = struct {
	ProfileID    primitive.ObjectID `json:"-" bson:"profileID"`
	ProfileType  string             `json:"-" bson:"profileType"`
	ID           string             // generated by controller
	UploadedTS   time.Time          `json:"UploadedTS" bson:"-"`
	UploadedID   primitive.ObjectID `json:"UploadedID" bson:"UploadedID"`
	UploadedName string             `json:"UploadedName" bson:"UploadedName"`
	SysFileName  string             `json:"-" bson:"fileName"`            // internal server file name
	OrigFileName string             `json:"fileName" bson:"origFileName"` // shown to client (uploader)
	Description  string             `json:"description" bson:"description,omitempty"`
	URL          string             `json:"url" bson:"-"`
	StatusCode   int32              `json:"statusCode" bson:"statusCD"` // will be using same code/status model as comments
	StatusText   string             `json:"statusText" bson:"-"`
	StatusTS     time.Time          `json:"statusTS" bson:"statusTS"`
	StatusID     primitive.ObjectID `json:"statusID" bson:"statusID"`
	StatusName   string             `json:"statusName" bson:"statusName"`
}

// UploadData is what's sent to the client (eg. Profile Picture or Screenshots)
// the structure is usually initialized by a model function, called by GetXXX
type UploadData = struct {
	URL        string `json:"URL"`
	StatusCode int32  `json:"statusCode"` // will be using same code/status model as comments
	StatusText string `json:"statusText"`
}

// UploadModel provides the logic to the interface and access to the database
type UploadModel struct {
	Client     *mongo.Client
	Collection *mongo.Collection
	// Gewisse Informationen kommen vom User-Model, die werden hier referenziert
	// somit muss das nicht der Controller machen
	GetUserName func(ID string) (string, error)
	// ToDo: halt umbennen GetCredentials
	CredentialsReader func(userId string, loadFriendlist bool) *Credentials
	GetUserVote       func(profileID string, userID string) (int32, error) // injected from vote model
}

// SaveMetaData
