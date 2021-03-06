package models

import (
	"context"
	"fmt"
	"forza-garage/apperror"
	"forza-garage/database"
	"forza-garage/helpers"
	"forza-garage/lookups"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Course is the "interface" used for client communication
type Course struct {
	// omitempty merkt selber, ob das feld im json vorhanden war :-) ohne wird def-wert des typs gespeichert
	// von angular käme dann wohl null von einem leeren control
	ID             primitive.ObjectID `json:"id" bson:"_id"`
	MetaInfo       Header             `json:"metaInfo" bson:"metaInfo"` // non-ptr = always present
	VisibilityCode int32              `json:"visibilityCode" bson:"visibilityCD"`
	VisibilityText string             `json:"visibilityText" bson:"-"`
	GameCode       int32              `json:"gameCode" bson:"gameCD"`
	GameText       string             `json:"gameText" bson:"-"`
	TypeCode       int32              `json:"typeCode" bson:"courseTypeCD"` // identifies object type (for searches, $exists)
	TypeText       string             `json:"typeText" bson:"-"`
	StyleCode      int32              `json:"styleCode" bson:"styleCD"` // circuit/sprint
	StyleText      string             `json:"styleText" bson:"-"`
	ForzaSharing   int32              `json:"forzaSharing" bson:"forzaSharing"` // sparse index in collection
	Name           string             `json:"name" bson:"name"`                 // same name as CMPs to enables over-all searches
	SeriesCode     int32              `json:"seriesCode" bson:"seriesCD"`
	SeriesText     string             `json:"seriesText" bson:"-"`
	CarClasses     []Lookup           `json:"carClassCodes" bson:"carClasses"` // multi-value lookups as nested structure
	Description    string             `json:"description" bson:"description,omitempty"`
	Route          *CourseRef         `json:"route" bson:"route,omitempty"` // standard route which a custom route is based on
	Tags           []string           `json:"tags" bson:"tags,omitempty"`
}

// CourseRef is used as a reference
type CourseRef struct {
	ID   primitive.ObjectID `json:"id" bson:"_id"`
	Name string             `json:"name" bson:"name"`
}

// CourseListItem is the reduced/simplified model used for listings
// ToDO: Allenfalls die UserVote auch integrieren (symbol in listen)
type CourseListItem struct {
	ID           primitive.ObjectID `json:"id"`
	CreatedTS    time.Time          `json:"createdTS"`
	CreatedID    primitive.ObjectID `json:"createdID"`
	CreatedName  string             `json:"createdName"`
	Rating       float32            `json:"rating"`
	GameCode     int32              `json:"gameCode"`
	GameText     string             `json:"gameText"`
	Name         string             `json:"name"`
	ForzaSharing int32              `json:"forzaSharing"`
	SeriesCode   int32              `json:"seriesCode"`
	SeriesText   string             `json:"seriesText"`
	StyleCode    int32              `json:"styleCode"`
	StyleText    string             `json:"styleText"`
	CarClasses   []Lookup           `json:"carClasses" bson:"carClasses"`
}

const (
	// CourseSearchModeAll includes both
	CourseSearchModeAll = 0
	// CourseSearchModeStandard returns standard routes (eg for look-ups/type-aheads)
	CourseSearchModeStandard = 1
	// CourseSearchModeCustom returns custom routes
	CourseSearchModeCustom = 2
)

// CourseSearchParams is passed as the search params
type CourseSearchParams struct {
	SearchMode  int // std/custom routes; Flags not a code
	GameCode    int32
	SeriesCodes []int32
	SearchTerm  string
	//Credentials *Credentials
}

/*
type CredentialsReader interface {
	GetCredentials(userId string) (*Credentials, error)
}*/

// CourseModel provides the logic to the interface and access to the database
type CourseModel struct {
	Client     *mongo.Client
	Collection *mongo.Collection
	// Gewisse Informationen kommen vom User-Model, die werden hier referenziert
	// somit muss das nicht der Controller machen
	GetUserName func(ID string) (string, error)
	// ToDo: halt umbennen GetCredentials
	CredentialsReader func(userId string, loadFriendlist bool) *Credentials
	GetUserVote       func(profileID string, userID string) (int32, error) // injected from vote model
}

// Models do not change original values passed by the controllers, but return new structures
// arguments (usually) passed by ref (pointers) for performance

// Validate checks given values and sets defaults where applicable (immutable)
func (m CourseModel) Validate(course Course) (*Course, error) {

	cleaned := course

	// ToDo:
	// Clean Strings
	// Validate Code Values (?) -> dann geht es nicxht mit Const/Enum, sondern const-array
	// ..according to model
	// Forza Share Code (Range)

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

	err := m.Collection.FindOne(ctx, bson.M{"forzaSharing": sharingCode}, options.FindOne().SetProjection(fields)).Decode(&data)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return false, nil
		}
		// treat errors as a "yes" - caller should not evaluate the result in case of an error
		return true, helpers.WrapError(err, helpers.FuncName())
	}
	// no error means a document was found, hence the object exists
	return true, nil
}

// CreateCourse adds a new route - validated by controller
func (m CourseModel) CreateCourse(course *Course, userID string) (string, error) {

	// set "system-fields"
	course.ID = primitive.NewObjectID()
	// course.MetaInfo.CreatedTS set by ID via OID
	course.MetaInfo.CreatedID = helpers.ObjectID(userID)
	userName, err := m.GetUserName(userID) // ToDo: Sollte direkt ObjhectID nehmen, 1 cast weniger
	if err != nil {
		// Fachlicher Fehler oder bereits wrapped
		return "", err
	}
	course.MetaInfo.CreatedName = userName // immer user name speichern, statisch
	course.MetaInfo.TouchedTS = time.Now()
	course.MetaInfo.Rating = 0
	course.MetaInfo.RecVer = 1
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
		return "", helpers.WrapError(err, helpers.FuncName()) // primitive.NilObjectID.Hex() ? probly useless
	}

	return res.InsertedID.(primitive.ObjectID).Hex(), nil
}

// SearchCourses lists or searches course (ohne Comments, aber mit Files/Tags)
// ACHTUNG: Die Liste wird sortiert und limitiert, daher können einzelne Dokumente herausfallen ;-)
func (m CourseModel) SearchCourses(searchSpecs *CourseSearchParams, userID string) ([]CourseListItem, error) {

	// CourseListeItem: Verkleinerte/vereinfachte Struktur für Listen
	// MongoDB muss eine passende Struktur erhalten um die Daten aufzunehmen (z. B. mit nested Arrays)
	// das API kann die Daten dann in die Listenstruktur kopieren
	// daher wird zum Aufnehmen der Daten aus der DB immer mit der Original-Struktur gearbeitet
	// Speicherbedarf bleibt halt gleich, dafür nimmt die Netzlast ab

	fields := bson.D{
		{Key: "_id", Value: 1},      // _id kommt immer, ausser es wird explizit ausgeschlossen (0)
		{Key: "metaInfo", Value: 1}, // {Key: "metaInfo.rating", Value: 1}, -- so könnte die nested struct eingeschränkt werden
		{Key: "gameCD", Value: 1},
		{Key: "name", Value: 1},
		{Key: "forzaSharing", Value: 1},
		{Key: "seriesCD", Value: 1},
		{Key: "styleCD", Value: 1},
		{Key: "carClasses", Value: 1},
	}

	sort := bson.D{
		{Key: "metaInfo.ratingSort", Value: -1},
		{Key: "metaInfo.rating", Value: -1},
		{Key: "metaInfo.touchedTS", Value: -1},
	}

	opts := options.Find().SetProjection(fields).SetLimit(20).SetSort(sort)

	// https://docs.mongodb.com/manual/tutorial/query-documents/
	// https://docs.mongodb.com/manual/reference/operator/query/#query-selectors
	// https://stackoverflow.com/questions/3305561/how-to-query-mongodb-with-like

	// perhaps, the searchTerm is a forza share code
	i, _ := strconv.Atoi(searchSpecs.SearchTerm)

	// construct a document containing the search parameters
	// filter := bson.D{}
	var filter bson.D

	// build IN-List of course types
	var courseTypes []int32
	switch searchSpecs.SearchMode {
	case CourseSearchModeAll:
		courseTypes = append(courseTypes, lookups.CourseTypeStandard)
		courseTypes = append(courseTypes, lookups.CourseTypeCustom)
	case CourseSearchModeStandard:
		courseTypes = append(courseTypes, lookups.CourseTypeStandard)
	case CourseSearchModeCustom:
		courseTypes = append(courseTypes, lookups.CourseTypeCustom)
	}

	credentials := m.CredentialsReader(userID, true)

	if credentials.RoleCode == lookups.UserRoleGuest {
		// anonymous visitors will only receive PUBLIC routes
		if searchSpecs.SearchTerm == "" {
			filter = bson.D{
				// every next field is AND
				{Key: "gameCD", Value: searchSpecs.GameCode}, // $eq kann wegelassen werden
				{Key: "courseTypeCD", Value: bson.D{ // selects courses rather than championships, just like $exists
					{Key: "$in", Value: courseTypes},
				}},
				{Key: "seriesCD", Value: bson.D{
					{Key: "$in", Value: searchSpecs.SeriesCodes},
				}},
				{Key: "visibilityCD", Value: lookups.VisibilityAll},
			}
			fmt.Println(filter)
		} else {
			filter = bson.D{
				// every next field is AND
				{Key: "gameCD", Value: searchSpecs.GameCode}, // $eq kann wegelassen werden
				{Key: "courseTypeCD", Value: bson.D{ // selects courses rather than championships, just like $exists
					{Key: "$in", Value: courseTypes},
				}},
				{Key: "seriesCD", Value: bson.D{
					{Key: "$in", Value: searchSpecs.SeriesCodes},
				}},
				{Key: "visibilityCD", Value: lookups.VisibilityAll},
				{Key: "$or", Value: bson.A{ // AND OR (...
					bson.D{{Key: "name", Value: primitive.Regex{Pattern: ".*" + searchSpecs.SearchTerm + ".*", Options: "/i"}}}, // LIKE %searchTerm% (case-insensitive)
					bson.D{{Key: "forzaSharing", Value: i}}, // 0 if searchTerm was alpha-numeric
				}},
			}
		}
	} else {
		// if a user is logged-in, check their privilidges (must be Admin or Member)
		//fmt.Printf("%s is logged in with role %v", credentials.LoginName, credentials.RoleCode)
		if credentials.RoleCode == lookups.UserRoleAdmin {
			// no visibility check needed for admins
			if searchSpecs.SearchTerm == "" {
				filter = bson.D{
					// every next field is AND
					{Key: "gameCD", Value: searchSpecs.GameCode}, // $eq kann wegelassen werden
					{Key: "courseTypeCD", Value: bson.D{ // selects courses rather than championships, just like $exists
						{Key: "$in", Value: courseTypes},
					}},
					{Key: "seriesCD", Value: bson.D{
						{Key: "$in", Value: searchSpecs.SeriesCodes},
					}},
					// visibility check removed
				}
			} else {
				// apply search Term
				filter = bson.D{
					// every next field is AND
					{Key: "gameCD", Value: searchSpecs.GameCode}, // $eq kann wegelassen werden
					{Key: "courseTypeCD", Value: bson.D{ // selects courses rather than championships, just like $exists
						{Key: "$in", Value: courseTypes},
					}},
					{Key: "seriesCD", Value: bson.D{
						{Key: "$in", Value: searchSpecs.SeriesCodes},
					}},
					// visibility check removed
					{Key: "$or", Value: bson.A{ // AND OR (...
						bson.D{{Key: "name", Value: primitive.Regex{Pattern: ".*" + searchSpecs.SearchTerm + ".*", Options: "/i"}}}, // LIKE %searchTerm% (case-insensitive)
						bson.D{{Key: "forzaSharing", Value: i}}, // 0 if searchTerm was alpha-numeric
					}},
				}
			}
		} else {
			// check visibility
			friendIDs := make([]primitive.ObjectID, len(credentials.Friends))
			for i, friend := range credentials.Friends {
				friendIDs[i] = friend.ReferenceID
			}

			if searchSpecs.SearchTerm == "" {
				filter = bson.D{
					// every next field is AND
					{Key: "gameCD", Value: searchSpecs.GameCode}, // $eq kann wegelassen werden
					{Key: "courseTypeCD", Value: bson.D{ // selects courses rather than championships, just like $exists
						{Key: "$in", Value: courseTypes},
					}},
					{Key: "seriesCD", Value: bson.D{
						{Key: "$in", Value: searchSpecs.SeriesCodes},
					}},
					// visibility check
					{Key: "$or", Value: bson.A{
						bson.D{{Key: "visibilityCD", Value: 0}},
						bson.D{{Key: "metaInfo.createdID", Value: credentials.UserID}},
						bson.D{{Key: "$and", Value: bson.A{
							bson.D{{Key: "visibilityCD", Value: 1}},
							bson.D{{Key: "metaInfo.createdID", Value: bson.D{{Key: "$in", Value: friendIDs}}}}, // nested doc for $in
						}}}, // nested $and-array im $or
					}}, // $or-array
				}
			} else {
				filter = bson.D{
					// every next field is AND
					{Key: "gameCD", Value: searchSpecs.GameCode}, // $eq kann wegelassen werden
					{Key: "courseTypeCD", Value: bson.D{ // selects courses rather than championships, just like $exists
						{Key: "$in", Value: courseTypes},
					}},
					{Key: "seriesCD", Value: bson.D{
						{Key: "$in", Value: searchSpecs.SeriesCodes},
					}},
					// visibility check
					{Key: "$or", Value: bson.A{
						bson.D{{Key: "visibilityCD", Value: 0}},
						bson.D{{Key: "metaInfo.createdID", Value: credentials.UserID}},
						bson.D{{Key: "$and", Value: bson.A{
							bson.D{{Key: "visibilityCD", Value: 1}},
							bson.D{{Key: "metaInfo.createdID", Value: bson.D{{Key: "$in", Value: friendIDs}}}}, // nested doc for $in
						}}}, // nested $and-array im $or
					}}, // $or-array
					// apply search term
					{Key: "$or", Value: bson.A{ // AND OR (...
						bson.D{{Key: "name", Value: primitive.Regex{Pattern: ".*" + searchSpecs.SearchTerm + ".*", Options: "/i"}}}, // LIKE %searchTerm% (case-insensitive)
						bson.D{{Key: "forzaSharing", Value: i}}, // 0 if searchTerm was alpha-numeric
					}}, // $or-array
				}
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel() // nach 10 Sekunden abbrechen

	cursor, err := m.Collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, helpers.WrapError(err, helpers.FuncName())
	}

	// receive results
	var courses []Course

	err = cursor.All(ctx, &courses)
	if err != nil {
		return nil, helpers.WrapError(err, helpers.FuncName())
	}

	// check for empty result set (no error raised by find)
	if courses == nil {
		return nil, apperror.ErrNoData
	}

	// copy data to reduced list-struct
	var courseList []CourseListItem
	var course CourseListItem

	for _, c := range courses {
		course.ID = c.ID
		course.CreatedTS = primitive.ObjectID.Timestamp(c.ID)
		course.CreatedID = c.MetaInfo.CreatedID
		course.CreatedName = c.MetaInfo.CreatedName
		course.Rating = c.MetaInfo.Rating
		course.GameCode = c.GameCode
		course.GameText = database.GetLookupText(lookups.LookupType(lookups.LTgame), c.GameCode)
		course.Name = c.Name
		course.ForzaSharing = c.ForzaSharing
		course.SeriesCode = c.SeriesCode
		course.SeriesText = database.GetLookupText(lookups.LookupType(lookups.LTseries), c.SeriesCode)
		course.StyleCode = c.StyleCode
		course.StyleText = database.GetLookupText(lookups.LookupType(lookups.LTcourseStyle), c.StyleCode)
		if len(c.CarClasses) > 0 {
			course.CarClasses = make([]Lookup, len(c.CarClasses))
			for i, v := range c.CarClasses {
				course.CarClasses[i].Value = v.Value
				course.CarClasses[i].Text = database.GetLookupText(lookups.LookupType(lookups.LTcarClass), v.Value)
			}
		}

		courseList = append(courseList, course)
	}

	return courseList, nil
}

// GetCourse returns one
func (m CourseModel) GetCourse(courseID string, userID string) (*Course, error) {
	//func (m CourseModel) GetCourse(courseID string, credentials *Credentials) (*Course, error) {

	id, err := primitive.ObjectIDFromHex(courseID)
	if err != nil {
		return nil, apperror.ErrNoData
	}

	data := Course{}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel() // nach 10 Sekunden abbrechen

	// später vielleicht project() wenn's zu viele felder werden (excl. nested oder sowas)
	err = m.Collection.FindOne(ctx, bson.M{"_id": id}).Decode(&data)
	if err != nil {
		return nil, apperror.ErrNoData
	}
	// extract creation timestamp from OID
	data.MetaInfo.CreatedTS = primitive.ObjectID(id).Timestamp()

	credentials := m.CredentialsReader(userID, true)

	err = GrantPermissions(data.VisibilityCode, data.MetaInfo.CreatedID, credentials)
	if err != nil {
		// no wrapping needed, since function returns app errors
		return nil, err
	}

	// get user's vote if present
	if userID != "" {
		// fehler kann hier ignoriert werden (default = 0 = note voted)
		uv, _ := m.GetUserVote(courseID, userID)
		data.MetaInfo.UserVote = uv
	}

	m.addLookups(&data)

	return &data, nil
}

// UpdateCourse modifies a given course
func (m CourseModel) UpdateCourse(course *Course, userID string) error {

	// read "metadata" to check permissions and perform optimistic locking
	// könnte eigentlich ausgelagert werden
	fields := bson.D{
		{Key: "_id", Value: 0},
		{Key: "metaInfo.createdID", Value: 1},
		{Key: "metaInfo.recVer", Value: 1},
		{Key: "visibilityCD", Value: 1},
	}

	filter := bson.D{{Key: "_id", Value: course.ID}}

	data := struct {
		CreatedID      primitive.ObjectID `bson:"metaInfo.createdID"`
		MetaInfo       Header             `bson:"metaInfo"` // declare & reserve entire nested object (seems required by driver)
		VisibilityCode int32              `bson:"visibilityCD"`
	}{}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel() // nach 10 Sekunden abbrechen

	// alternative schreibweisen:
	//err := m.Collection.FindOne(ctx, bson.D{{Key: "_id", Value: course.ID}}, options.FindOne().SetProjection(fields)).Decode(&data)
	//err := m.Collection.FindOne(ctx, bson.M{"_id": course.ID}, options.FindOne().SetProjection(fields)).Decode(&data)

	err := m.Collection.FindOne(ctx, filter, options.FindOne().SetProjection(fields)).Decode(&data)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return apperror.ErrNoData // document might have been deleted
		}
		// pass any other error
		return helpers.WrapError(err, helpers.FuncName())
	}

	/*
		if (data.CreatedID != credentials.UserID) && (credentials.RoleCode < lookups.UserRoleAdmin) {
			fmt.Println("test1")
			return ErrDenied
		}
	*/

	credentials := m.CredentialsReader(userID, false)

	// ToDO: GrantPermission für Course-Klasse erstellen
	err = GrantPermissions(data.VisibilityCode, data.CreatedID, credentials)
	if err != nil {
		// no wrapping needed, since function returns app errors
		return err
	}

	// optimistic lock check
	if data.MetaInfo.RecVer != course.MetaInfo.RecVer {
		// document was changed by another user since last read
		return apperror.ErrRecordChanged
	}

	// ToDO: einzel-upd wohl besser, oder gar replace?
	// replace nicht "nachhaltig" wenn bspw. Arrays/Nesteds drin sind, die gar nicht immer gelesen werden
	// definition für den moment: "alle änderbaren" felder halt neu setzen
	// arrays somit ersetzen, oder in spez. services ändern (z. B. add friends, falls embedded)

	// set "systemfields"
	course.MetaInfo.ModifiedID = credentials.UserID
	course.MetaInfo.ModifiedName = credentials.LoginName
	//now := time.Now()
	course.MetaInfo.ModifiedTS = time.Now()
	course.MetaInfo.TouchedTS = course.MetaInfo.ModifiedTS

	// set fields to be possibily updated
	fields = bson.D{
		// systemfields
		{Key: "$set", Value: bson.D{{Key: "metaInfo.modifiedTS", Value: course.MetaInfo.ModifiedTS}}},
		{Key: "$set", Value: bson.D{{Key: "metaInfo.modifiedID", Value: course.MetaInfo.ModifiedID}}},
		{Key: "$set", Value: bson.D{{Key: "metaInfo.modifiedName", Value: course.MetaInfo.ModifiedName}}},
		{Key: "$set", Value: bson.D{{Key: "metaInfo.touchedTS", Value: course.MetaInfo.TouchedTS}}},
		{Key: "$inc", Value: bson.D{{Key: "metaInfo.recVer", Value: 1}}}, // increase record version no
		// payload
		{Key: "$set", Value: bson.D{{Key: "visibilityCD", Value: course.VisibilityCode}}},
		{Key: "$set", Value: bson.D{{Key: "gameCD", Value: course.GameCode}}},
		// typeCode is static
		{Key: "$set", Value: bson.D{{Key: "forzaSharing", Value: course.ForzaSharing}}},
		{Key: "$set", Value: bson.D{{Key: "name", Value: course.Name}}},
		{Key: "$set", Value: bson.D{{Key: "seriesCD", Value: course.SeriesCode}}},
		{Key: "$set", Value: bson.D{{Key: "styleCD", Value: course.StyleCode}}},
		{Key: "$set", Value: bson.D{{Key: "carClasses", Value: course.CarClasses}}}, // arrays replaced as a whole
		{Key: "$set", Value: bson.D{{Key: "description", Value: course.Description}}},
		{Key: "$set", Value: bson.D{{Key: "tags", Value: course.Tags}}},
	}

	result, err := m.Collection.UpdateOne(ctx, filter, fields)
	if err != nil {
		return helpers.WrapError(err, helpers.FuncName())
	}

	if result.MatchedCount == 0 {
		return apperror.ErrNoData // document might have been deleted
	}

	// ToDO: überlegen - rückgsabewerte sinnvoll? (z. B. timestamp? oder die ID analog add?)
	return nil
}

// SetRating is called by the voting model
func (m CourseModel) SetRating(social *Social) error {

	// set fields to be possibily updated
	fields := bson.D{
		// systemfields
		{Key: "$set", Value: bson.D{{Key: "metaInfo.rating", Value: social.Rating}}},
		{Key: "$set", Value: bson.D{{Key: "metaInfo.ratingSort", Value: social.SortOrder}}},
		{Key: "$set", Value: bson.D{{Key: "metaInfo.upVotes", Value: social.UpVotes}}},
		{Key: "$set", Value: bson.D{{Key: "metaInfo.downVotes", Value: social.DownVotes}}},
		{Key: "$set", Value: bson.D{{Key: "metaInfo.touchedTS", Value: social.TouchedTS}}},
	}

	filter := bson.D{{Key: "_id", Value: social.ProfileOID}}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel() // nach 10 Sekunden abbrechen

	result, err := m.Collection.UpdateOne(ctx, filter, fields)
	if err != nil {
		return helpers.WrapError(err, helpers.FuncName())
	}

	if result.MatchedCount == 0 {
		return apperror.ErrNoData // document might have been deleted
	}

	return nil
}

// internal helpers (private methods)

// actually that's not immutable, but ok here
func (m CourseModel) addLookups(course *Course) *Course {
	course.VisibilityText = database.GetLookupText(lookups.LookupType(lookups.LTvisibility), course.VisibilityCode)
	course.GameText = database.GetLookupText(lookups.LookupType(lookups.LTgame), course.GameCode)
	course.TypeText = database.GetLookupText(lookups.LookupType(lookups.LTcourseType), course.TypeCode)
	course.SeriesText = database.GetLookupText(lookups.LookupType(lookups.LTseries), course.SeriesCode)
	course.StyleText = database.GetLookupText(lookups.LookupType(lookups.LTcourseStyle), course.StyleCode)
	for i, v := range course.CarClasses {
		course.CarClasses[i].Text = database.GetLookupText(lookups.LookupType(lookups.LTcarClass), v.Value)
	}

	return course // müsste gar nichts zurückliefern ;-)
}
