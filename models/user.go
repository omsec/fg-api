package models

import (
	"context"
	"forza-garage/database"
	"forza-garage/helpers"
	"forza-garage/lookups"
	"sort"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// User is the "interface" used for client communication
type User struct {
	ID           primitive.ObjectID `json:"id" bson:"_id"`
	LoginName    string             `json:"loginName" bson:"loginName"` // unique
	Password     string             `json:"password" bson:"password"`   // hash value
	RoleCode     int32              `json:"roleCode" bson:"roleCD"`
	RoleText     string             `json:"roleText" bson:"-"`
	LanguageCode int32              `json:"languageCode" bson:"languageCD" header:"Language"`
	LanguageText string             `json:"languageText" bson:"-"`
	EMailAddress string             `json:"eMail" bson:"eMail"`                 // unique
	XBoxTag      string             `json:"XBoxTag" bson:"XBoxTag"`             // unique
	LastSeenTS   []time.Time        `json:"lastSeen" bson:"lastSeen,omitempty"` // limited to 5 in DB-Query (setLastSeen)
	Friends      []UserRef          `json:"friends" bson:"-"`                   // loaded from diff. collection
	Following    []UserRef          `json:"following" bson:"-"`                 // loaded from diff. collection
	Followers    []UserRef          `json:"followers" bson:"-"`                 // loaded from diff. collection
	// ToDo: []LastPasswords - check for 90 days or 10 entries
}

// Credentials is used for programmatic control
// non-ptr values require annotations!
type Credentials struct {
	UserID       primitive.ObjectID
	LoginName    string
	RoleCode     int32 `bson:"roleCD"`
	LanguageCode int32 `bson:"languageCD"` // ToDo: Lesen aus Header für ANONYM, DB für Members (override-Möglichkeit)
	Friends      []UserRef
}

// UserRef is a simple reference to something (another user as a friend or follower) or an object as an "observable"
type UserRef struct {
	UserID        primitive.ObjectID `json:"userID" bson:"userID"` // referencing user
	UserName      string             `json:"userName" bson:"userName"`
	ReferenceID   primitive.ObjectID `json:"referenceID" bson:"refID"`     // referenced ID
	ReferenceName string             `json:"referenceName" bson:"refName"` // name of referenced user/object
	ReferenceType string             `json:"referenceType" bson:"refType"` // user, course, championship etc.
	// eigentlich hier unnötig, aber einfacher
	RelationType string `json:"-" bson:"relType"` // friend, following/observing, follower
}

// UserModel provides the logic to the interface and access to the database
// (assigned in initialization of the controller)
type UserModel struct {
	Client *mongo.Client
	// could be a map - overkill ;-)
	Collection *mongo.Collection
	Social     *mongo.Collection
}

// UserExists checks if a User Name is available - used in client for in-type error checking
// (wrapper of internal helper function)
func (m UserModel) UserExists(userName string) bool {
	b, _ := userExists(m.Collection, userName)
	return b
}

// EMailAddressExists checks if an eMail-Address is already assigned with any User Name
// used in client for in-type error checking
func (m UserModel) EMailAddressExists(eMailAddress string) bool {
	b, _ := eMailExists(m.Collection, eMailAddress)
	return b
}

// CreateUser adds a new User
func (m UserModel) CreateUser(user User) (string, error) {

	var err error

	// ToDO: Validate (ext fnc)

	// ToDo: entfernen => PK in DB machen
	// braucht Hilfs-Proc (package DB) um die MSG zu parsen key: ....
	// "multiple write errors: [{write errors: [{E11000 duplicate key error collection: forzaGarage.users index: loginName_1 dup key: { loginName: \"roger\" }}]}, {<nil>}]"
	// https://stackoverflow.com/questions/56916969/with-mongodb-go-driver-how-do-i-get-the-inner-exceptions
	b, err := userExists(m.Collection, user.LoginName)
	if b || err != nil {
		return "", ErrUserNameNotAvailable
	}

	// ToDo: entfernen => PK in DB machen
	b, err = eMailExists(m.Collection, user.EMailAddress)
	if b || err != nil {
		return "", ErrEMailAddressTaken
	}

	pwdHash, err := helpers.GenerateHash(user.Password)
	if err != nil {
		return "", helpers.WrapError(err, helpers.FuncName())
	}

	user.ID = primitive.NewObjectID()
	user.Password = pwdHash
	user.RoleCode = lookups.UserRoleGuest
	user.LastSeenTS = append(user.LastSeenTS, time.Now())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel() // nach 10 Sekunden abbrechen

	res, err := m.Collection.InsertOne(ctx, user)
	if err != nil {
		return "", helpers.WrapError(err, helpers.FuncName()) // primitive.NilObjectID.Hex() ? probly useless
	}

	return res.InsertedID.(primitive.ObjectID).Hex(), nil
}

// GetUserByName reads a user's login account data
func (m UserModel) GetUserByName(userName string) (*User, error) {

	var err error
	var user User

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel() // nach 10 Sekunden abbrechen

	err = m.Collection.FindOne(ctx, bson.M{"loginName": userName}).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrInvalidUser
		}
		// pass any other error
		return nil, helpers.WrapError(err, helpers.FuncName())
	}

	// ToDO: überlegen, ob hier die Friends etc. gelesen werden sollen - denke nicht nötig (getcredeitnaisl für prüfungen, sonst getProfile...?)

	// add look-up texts
	addLookups(&user)

	return &user, nil
}

// GetUserByID reads a user's login account data
func (m UserModel) GetUserByID(ID string) (*User, error) {

	var user User

	// https://ildar.pro/golang-hints-create-mongodb-object-id-from-string/
	id, err := primitive.ObjectIDFromHex(ID)
	if err != nil {
		return nil, ErrNoData // eigentlich ErrInvalidUser da keine gültige OID, jedoch std-meldung ausgeben
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel() // nach 10 Sekunden abbrechen

	err = m.Collection.FindOne(ctx, bson.M{"_id": id}).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrNoData
		}
		// pass any other error
		return nil, helpers.WrapError(err, helpers.FuncName())
	}

	// add look-up text
	//user.RoleText = database.GetLookupText(lookups.LookupType(lookups.LTuserRole), user.RoleCode)
	addLookups(&user)

	return &user, nil
}

// GetUserName returns the login name from an ID (reduced version, without profile data)
func (m UserModel) GetUserName(ID string) (string, error) {

	data := struct {
		//ID        primitive.ObjectID `bson:"_id"`
		LoginName string `bson:"loginName"`
	}{}

	id, err := primitive.ObjectIDFromHex(ID)
	if err != nil {
		return "", ErrInvalidUser
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel() // nach 10 Sekunden abbrechen

	fields := bson.D{
		{Key: "_id", Value: 0}, // _id kommt immer, ausser es wird explizit ausgeschlossen (0)
		{Key: "loginName", Value: 1}}

	err = m.Collection.FindOne(ctx, bson.M{"_id": id}, options.FindOne().SetProjection(fields)).Decode(&data)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return "", ErrInvalidUser
		}
		// pass any other error
		return "", helpers.WrapError(err, helpers.FuncName())

	}

	return data.LoginName, nil
}

// CheckPassword tests if a login's password matches
// (kein DB-Zugriff nötig)
func (m UserModel) CheckPassword(givenPassword string, userInfo User) bool {
	match, err := helpers.CompareHash(userInfo.Password, givenPassword)
	if err != nil {
		return false
	}
	return match
}

// SetLastSeen saves timestamp of last log-in
// Rolling Window
// https://stackoverflow.com/questions/29932723/how-to-limit-an-array-size-in-mongodb
// ToDo: add IP-Address & record history (collection analytics)
func (m UserModel) SetLastSeen(userID primitive.ObjectID) {
	// no error is returned since this function is not essential

	filter := bson.D{{Key: "_id", Value: userID}}
	//update := bson.D{{Key: "$set", Value: bson.D{{Key: "lastSeenTS", Value: time.Now()}}}}
	update := bson.M{
		"$push": bson.M{
			"lastSeen": bson.M{
				"$each":  bson.A{time.Now()},
				"$slice": -5, // keep 5 last items in this array
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel() // nach 10 Sekunden abbrechen

	// just fire & forget
	_, _ = m.Collection.UpdateOne(ctx, filter, update)
}

// SetPassword is used to change a User's password
func (m UserModel) SetPassword(userID primitive.ObjectID, newPassword string) error {
	// ToDO: call PWD-Validator func

	pwdHash, err := helpers.GenerateHash(newPassword)
	if err != nil {
		return helpers.WrapError(err, helpers.FuncName())
	}

	filter := bson.D{{Key: "_id", Value: userID}}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "password", Value: pwdHash}}}}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel() // nach 10 Sekunden abbrechen

	result, err := m.Collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return helpers.WrapError(err, helpers.FuncName())
	}

	// just an additional check to discover data consistency problems
	if result.MatchedCount != 1 || result.ModifiedCount != 1 {
		// treat this as system error (which causes 500)
		return helpers.WrapError(ErrMultipleRecords, helpers.FuncName())
	}

	return nil
}

// GetCredentials returns account infos to control permissions and text-out (language)
func (m UserModel) GetCredentials(UserID string) (*Credentials, error) {
	var credentials Credentials

	if UserID == "" {
		credentials.UserID = primitive.NilObjectID
		// anonymous (visitor) receives default role
		credentials.RoleCode = lookups.UserRoleGuest
	} else {
		// look-up credentials in database

		// https://ildar.pro/golang-hints-create-mongodb-object-id-from-string/
		id, err := primitive.ObjectIDFromHex(UserID)
		if err != nil {
			return nil, ErrInvalidUser
		}

		fields := bson.D{
			{Key: "_id", Value: 0}, // _id kommt immer, ausser es wird explizit ausgeschlossen (0)
			{Key: "loginName", Value: 1},
			{Key: "roleCD", Value: 1}, // {Key: "metaInfo.rating", Value: 1}, -- so könnte die nested struct eingeschränkt werden
			{Key: "languageCD", Value: 1},
		}

		opts := options.FindOne().SetProjection(fields)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel() // nach 10 Sekunden abbrechen

		err = m.Collection.FindOne(ctx, bson.M{"_id": id}, opts).Decode(&credentials)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				return nil, ErrInvalidUser
			}
			// pass any other error
			return nil, helpers.WrapError(err, helpers.FuncName())
		}
		credentials.UserID = id

		// friendlist ist referenced from its own collection, add it
		credentials.Friends, err = m.GetFriends(UserID)
		if err != nil {
			// "no data/no friends" is not an error, other errors are already wrapped
			if err != ErrNoData {
				return nil, err
			}
		}
	}

	return &credentials, nil
}

// GetFriends lists all friends of a user
func (m UserModel) GetFriends(userID string) ([]UserRef, error) {
	// cal private proc

	return m.getReferences(userID, "friend")
}

// GetFollowings lists all users someone (the userID) is following
func (m UserModel) GetFollowings(userID string) ([]UserRef, error) {
	// cal private proc

	return m.getReferences(userID, "following")
}

// GetFollowers lists all users who are following someone (the userID)
func (m UserModel) GetFollowers(userID string) ([]UserRef, error) {
	// cal private proc

	return m.getReferences(userID, "follower")
}

// AddFriend adds another user to the friendlist
func (m UserModel) AddFriend(userID string, friendUserID string) error {
	// ToDO: Mehrere auf einmal unterstüzen?

	if userID == friendUserID {
		return ErrInvalidFriend
	}

	// objectID required for update
	userOID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return ErrInvalidUser
	}

	friendOID, err := primitive.ObjectIDFromHex(friendUserID)
	if err != nil {
		return ErrInvalidUser
	}

	// provisorisch
	userInfo, err := m.GetCredentials(userID)
	if err != nil {
		return err
	}

	friendInfo, err := m.GetCredentials(friendUserID)
	if err != nil {
		return err
	}

	// ein eintrag ist genug, da diese beziehungen nicht gerichtet (wie bspw. Vormund/Mündel) sind
	// somit entfallen teure Transaktionen
	data := UserRef{
		UserID:        userOID,
		UserName:      userInfo.LoginName,
		ReferenceID:   friendOID,
		ReferenceName: friendInfo.LoginName,
		ReferenceType: "user",
		RelationType:  "friend"}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel() // nach 10 Sekunden abbrechen

	_, err = m.Social.InsertOne(ctx, data)
	if err != nil {
		return helpers.WrapError(err, helpers.FuncName())
	}

	return nil
}

// RemoveFriend deletes a user from the friendlist
func (m UserModel) RemoveFriend(userID string, friendUserID string) error {
	return nil
}

// FollowUser "registers" a user to follow another user
func (m UserModel) FollowUser(userID string, followUserID string) error {
	// ToDO: Mehrere auf einmal unterstüzen?

	if userID == followUserID {
		return ErrInvalidFriend
	}

	// objectID required for update
	userOID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return ErrInvalidUser
	}

	followOID, err := primitive.ObjectIDFromHex(followUserID)
	if err != nil {
		return ErrInvalidUser
	}

	// provisorisch
	userInfo, err := m.GetCredentials(userID)
	if err != nil {
		return err
	}

	followInfo, err := m.GetCredentials(followUserID)
	if err != nil {
		return err
	}

	// ein eintrag ist genug, da diese beziehungen nicht gerichtet (wie bspw. Vormund/Mündel) sind
	// somit entfallen teure Transaktionen
	data := UserRef{
		UserID:        userOID,
		UserName:      userInfo.LoginName,
		ReferenceID:   followOID,
		ReferenceName: followInfo.LoginName,
		ReferenceType: "user",
		RelationType:  "following"}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel() // nach 10 Sekunden abbrechen

	_, err = m.Social.InsertOne(ctx, data)
	if err != nil {
		return helpers.WrapError(err, helpers.FuncName())
	}

	return nil
}

// private proc to read relations/referenced documents, such as friends
func (m UserModel) getReferences(userID string, relationType string) ([]UserRef, error) {
	// TODO: Validate inparams

	userOID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil, ErrInvalidUser
	}

	fields := bson.M{
		"_id":      0,
		"userID":   1,
		"userName": 1,
		"refID":    1,
		"refName":  1,
	}

	// actually not required for friendlist, due do post-processing (sort on slice)
	dbSort := bson.M{
		"userName": 1,
	}

	// ToDo: Limit sinnvoll?
	opts := options.Find().SetProjection(fields).SetLimit(20).SetSort(dbSort)

	// different query depending on relation type
	var filter bson.M

	switch relationType {
	case "friend":
		filter = bson.M{
			"relType": relationType,
			"$or": bson.A{
				bson.M{"userID": userOID},
				bson.M{"refID": userOID},
			},
		}
	case "following":
		// wem folge ich? abfrage auf db.userID = userID
		filter = bson.M{
			"relType": relationType,
			"userID":  userOID,
		}
	case "follower":
		// wer folgt mir? abfrage auf refID = userID
		filter = bson.M{
			"relType": "following", // same type/verb, different context
			"refID":   userOID,
		}
	case "observing":
		// welche rat/cmp etc. beobachte ich?
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel() // nach 10 Sekunden abbrechen

	cursor, err := m.Social.Find(ctx, filter, opts)
	if err != nil {
		return nil, helpers.WrapError(err, helpers.FuncName())
	}

	// receive results
	var results []UserRef

	err = cursor.All(ctx, &results)
	if err != nil {
		return nil, helpers.WrapError(err, helpers.FuncName())
	}

	// check for empty result set (no error raised by find)
	if results == nil {
		return nil, ErrNoData
	}

	// final list
	var reference UserRef
	var references []UserRef
	// storing 2 docs and use a trx makes it easier, but takes more disk space (=pay!)
	// if userID = res.userID -> res.refID
	// if userID = res.refID -> res.userID

	if relationType == "friend" {
		for _, r := range results {
			reference.UserID = userOID
			if r.UserID == userOID {
				reference.UserName = r.UserName
				reference.ReferenceID = r.ReferenceID
				reference.ReferenceName = r.ReferenceName
			} else {
				reference.UserName = r.ReferenceName
				reference.ReferenceID = r.UserID
				reference.ReferenceName = r.UserName
			}
			reference.ReferenceType = "user"
			reference.ReferenceType = relationType

			references = append(references, reference)
		}
		// https://zetcode.com/golang/sort/
		sort.Slice(references, func(i, j int) bool {
			return references[i].ReferenceName < references[j].ReferenceName
		})
	}

	if relationType == "following" {
		for _, r := range results {
			reference.UserID = r.UserID
			reference.UserName = r.UserName
			reference.ReferenceID = r.ReferenceID
			reference.ReferenceName = r.ReferenceName
			reference.ReferenceType = "user"
			reference.RelationType = relationType

			references = append(references, reference)
		}
	}

	if relationType == "follower" {
		for _, r := range results {
			reference.UserID = r.ReferenceID
			reference.UserName = r.ReferenceName
			reference.ReferenceID = r.UserID
			reference.ReferenceName = r.UserName
			reference.ReferenceType = "user"
			reference.RelationType = relationType

			references = append(references, reference)
		}
	}

	return references, nil
}

// public "static" methods

// UserReferenced scans a slice for a given item
func UserReferenced(slice []UserRef, val primitive.ObjectID) bool {
	for _, item := range slice {
		if item.UserID == val {
			return true
		}
	}
	return false
}

// GrantPermissions enforces access rights
func GrantPermissions(itemVisibilityCode int32, itemCreatorID primitive.ObjectID, credentials *Credentials) error {

	if credentials.RoleCode == lookups.UserRoleAdmin {
		return nil
	}

	if itemVisibilityCode == lookups.VisibilityMembers && credentials.RoleCode == lookups.UserRoleGuest {
		// get a log-in and make friends
		return ErrGuest
	}

	if (itemVisibilityCode == lookups.VisibilityMembers) && (UserReferenced(credentials.Friends, itemCreatorID) == false) && (itemCreatorID != credentials.UserID) {
		// make friends with them
		return ErrNotFriend
	}

	if itemVisibilityCode == lookups.VisibilityNone && (credentials.UserID != itemCreatorID) {
		// ask them to share
		return ErrPrivate
	}

	// all checks passed
	return nil
}

// internal implementations that are used by multiple methods of the model and corresponding handlers
// ToDo: müsste nicht ausgelagert sein, kann private member sein (klein schreiben)

func userExists(collection *mongo.Collection, userName string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel() // nach 10 Sekunden abbrechen

	// there seems to be no function like "exists" so a projection on just the ID is used
	fields := bson.D{
		{Key: "_id", Value: 1}}

	data := struct {
		ID primitive.ObjectID `bson:"_id"`
	}{}

	// some (old) sources say FindOne is slow and we should use find instead... (?)
	err := collection.FindOne(ctx, bson.M{"loginName": userName}, options.FindOne().SetProjection(fields)).Decode(&data)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return false, nil
		}
		// treat errors as a "yes" - caller should not evaluate the result in case of an error
		return true, helpers.WrapError(err, helpers.FuncName())
	}
	// no error means a document was found, hence the user does exist
	return true, nil
}

func eMailExists(collection *mongo.Collection, emailAddress string) (bool, error) {
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
	err := collection.FindOne(ctx, bson.M{"eMail": emailAddress}, options.FindOne().SetProjection(fields)).Decode(&data)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return false, nil
		}
		// treat errors as a "yes" - caller should not evaluate the result in case of an error
		return true, helpers.WrapError(err, helpers.FuncName())
	}
	// no error means a document was found, hence the user does exist
	return true, nil
}

// internal helpers

// actually that's not immutable, but ok here
func addLookups(user *User) *User {
	user.RoleText = database.GetLookupText(lookups.LookupType(lookups.LTuserRole), user.RoleCode)
	user.LanguageText = database.GetLookupText(lookups.LookupType(lookups.LTlang), user.LanguageCode)

	return user
}
