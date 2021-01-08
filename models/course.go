package models

import (
	"context"
	"errors"
	"forza-garage/database"
	"forza-garage/lookups"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ToDo: UI on Forza Sharing Code
// https://docs.mongodb.com/manual/core/index-unique/

// Course is the "interface" used for client communication
type Course struct {
	ID             primitive.ObjectID `json:"id" bson:"_id"`
	MetaInfo       Header             `json:"metaInfo" bson:"metaInfo"`
	VisibilityCode int32              `json:"visibilityCode" bson:"visibilityCD"`
	VisibilityText string             `json:"visibilityText" bson:"-"`
	TypeCode       int32              `json:"typeCode" bson:"courseTypeCD"` // identifies object type (for searches, $exists)
	TypeText       string             `json:"typeText" bson:"-"`
	ForzaSharing   int32              `json:"forzaSharing" bson:"forzaSharing"`
	Name           string             `json:"name" bson:"name"` // same name as CMPs to enables over-all searches
	SeriesCode     int32              `json:"seriesCode" bson:"seriesCD"`
	SeriesText     string             `json:"seriesText" bson:"-"`
	CarClassCode   int32              `json:"carClassCode" bson:"carClassCD"`
	CarClassText   string             `json:"carClassText" bson:"-"`
}

// CourseListItem is the reduced/simplified model used for listings
type CourseListItem struct {
	ID           primitive.ObjectID `json:"id"`
	CreatedID    primitive.ObjectID `json:"createdID"`
	CreatedName  string             `json:"createdName"`
	Rating       float32            `json:"rating"`
	Name         string             `json:"name"`
	ForzaSharing int32              `json:"forzaSharing"`
	SeriesCode   int32              `json:"seriesCode"`
	SeriesText   string             `json:"seriesText"`
	CarClassCode int32              `json:"carClassCode"`
	CarClassText string             `json:"carClassText"`
}

// CourseModel provides the logic to the interface and access to the database
type CourseModel struct {
	Client     *mongo.Client
	Collection *mongo.Collection
}

// custom error types
var (
	ErrForzaSharingCodeMissing = errors.New("sharing code is required")
	ErrCourseNameMissing       = errors.New("course name is required")
	ErrSeriesMissing           = errors.New("series is required")
	ErrForzaSharingCodeTaken   = errors.New("forza sharing code already used")
)

// Validate checks given values and sets defaults where applicable (immutable)
func (m CourseModel) Validate(course Course) (*Course, error) {

	cleaned := course

	// ToDo:
	// Clean Strings
	// Validate Code Values (?) -> dann geht es nicxht mit Const/Enum, sondern const-array
	// ..according to model

	cleaned.Name = strings.TrimSpace(cleaned.Name)
	if course.Name == "" {
		return nil, ErrCourseNameMissing
	}

	return &cleaned, nil
}

// ForzaSharingExists checks if a "Sharing Code" in the game already exists (which is their PK)
// this is used for in-line validation while typing in the client's form
func (m CourseModel) ForzaSharingExists(sharingCode int32) (bool, error) {

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel() // nach 10 Sekunden abbrechen

	// there seems to be no function like "exists" so a projection on just the ID is used
	fields := bson.D{
		{Key: "_id", Value: 1}}

	data := struct {
		ID primitive.ObjectID `bson:"_id"`
	}{}

	// some (old) sources say FindOne is slow and we should use find instead... (?)
	// ToDO: Add index to field in MongoDB
	err := m.Collection.FindOne(ctx, bson.M{"forzaSharing": sharingCode}, options.FindOne().SetProjection(fields)).Decode(&data)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return false, nil
		}
		// treat errors as a "yes" - caller should not evaluate the result in case of an error
		return true, err
	}
	// no error means a document was found, hence the object exists
	return true, nil
}

// CreateCourse adds a new route - validated by controller
// ToDO: Rename "Add" ?
func (m CourseModel) CreateCourse(course *Course) (string, error) {

	// set "system-fields"
	course.ID = primitive.NewObjectID()
	course.MetaInfo.TouchedTS = time.Now()
	course.MetaInfo.Rating = 0
	course.MetaInfo.RecVer = 0
	course.TypeCode = lookups.CourseTypeCustom

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel() // nach 10 Sekunden abbrechen

	res, err := m.Collection.InsertOne(ctx, course)
	if err != nil {
		// leider können DB-Error Codes nicht direkt aus dem Fehler ausgelesen werden
		// https://stackoverflow.com/questions/56916969/with-mongodb-go-driver-how-do-i-get-the-inner-exceptions

		if (err.(mongo.WriteException).WriteErrors[0].Code) == 11000 {
			// Error 11000 = DUP
			// since there is only one unique index in the collection, it's a duplicate forza share code
			return "", ErrForzaSharingCodeTaken
		}
		// any other error
		return "", err // primitive.NilObjectID.Hex() ? probly useless
	}

	return res.InsertedID.(primitive.ObjectID).Hex(), nil
}

// SearchCourses lists or searches course (ohne Comments, aber mit Files/Tags)
func (m CourseModel) SearchCourses(searchTerm string) ([]CourseListItem, error) {

	// ToDo: arr user-Struct as param (reduced) and check credentials
	// filter except for admins
	// -> ohne userId nicht prüfen (filter public)

	// Verkleinerte/vereinfachte Struktur für Listen
	// MongoDB muss eine passende Struktur erhalten um die Daten aufzunehmen (z. B. mit nested Arrays)
	// das API kann die Daten dann in die Listenstruktur kopieren
	// daher wird zum Aufnehmen der Daten aus der DB immer mit der Original-Struktur gearbeitet
	// Speicherbedarf bleibt halt gleich, dafür nimmt die Netzlast ab

	// use original struct to receive selected fields
	fields := bson.D{
		{Key: "_id", Value: 1},      // _id kommt immer, ausser es wird explizit ausgeschlossen (0)
		{Key: "metaInfo", Value: 1}, // {Key: "metaInfo.rating", Value: 1}, -- so könnte die nested struct eingeschränkt werden
		{Key: "name", Value: 1},
		{Key: "forzaSharing", Value: 1},
		{Key: "seriesCD", Value: 1},
		{Key: "carClassCD", Value: 1},
	}

	sort := bson.D{
		{Key: "metaInfo.rating", Value: -1},
		{Key: "metaInfo.touchedTS", Value: -1},
	}

	opts := options.Find().SetProjection(fields).SetLimit(20).SetSort(sort)

	// only list documents of "course" type
	// ToDO: Visibility Code - überlegen, wie das mit offenen benutzer (ohne login) sein soll - glaube macht so keinen Sinn mehr => all/friends

	// https://docs.mongodb.com/manual/tutorial/query-documents/
	// https://docs.mongodb.com/manual/reference/operator/query/#query-selectors
	// https://stackoverflow.com/questions/3305561/how-to-query-mongodb-with-like

	// perhaps, the searchTerm is a forza share code
	i, _ := strconv.Atoi(searchTerm)

	filter := bson.D{}
	// use a simple & efficient query to return everything
	if searchTerm == "" {
		filter = bson.D{
			{Key: "courseTypeCD", Value: bson.D{{Key: "$exists", Value: "true"}}}, // return std and community courses
		}
	} else {
		filter = bson.D{
			{Key: "courseTypeCD", Value: bson.D{{Key: "$exists", Value: "true"}}}, // look for courses, next conditions will be AND (then OR)
			{Key: "$or", Value: bson.A{
				//bson.D{{Key: "name", Value: bson.D{{Key: "$eq", Value: searchTerm}}}},
				bson.D{{Key: "name", Value: primitive.Regex{Pattern: ".*" + searchTerm + ".*", Options: "/i"}}}, // LIKE %searchTerm% (case-insensitive)
				bson.D{{Key: "forzaSharing", Value: bson.D{{Key: "$eq", Value: i}}}},                            // 0 if searchTerm was alpha-numeric
			}},
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel() // nach 10 Sekunden abbrechen

	cursor, err := m.Collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}

	var courses []Course

	err = cursor.All(ctx, &courses)
	if err != nil {
		return nil, err
	}

	// check for empty result set (no error raised by find)
	if courses == nil {
		return nil, ErrNoData
	}

	// copy data to reduced list-struct
	var courseList []CourseListItem
	var course CourseListItem

	for _, v := range courses {
		course.ID = v.ID
		course.CreatedID = v.MetaInfo.CreatedID
		course.CreatedName = v.MetaInfo.CreatedName
		course.Rating = v.MetaInfo.Rating
		course.Name = v.Name
		course.ForzaSharing = v.ForzaSharing
		course.SeriesCode = v.SeriesCode
		course.SeriesText = database.GetLookupText(lookups.LookupType(lookups.LTseries), v.SeriesCode)
		course.CarClassCode = v.CarClassCode
		course.CarClassText = database.GetLookupText(lookups.LookupType(lookups.LTcarClass), v.CarClassCode)

		courseList = append(courseList, course)
	}

	return courseList, nil
}