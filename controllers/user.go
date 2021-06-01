package controllers

import (
	"fmt"
	"forza-garage/apperror"
	"forza-garage/authentication"
	"forza-garage/environment"
	"forza-garage/helpers"
	"forza-garage/lookups"
	"forza-garage/models"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/twinj/uuid"
)

// Test is what it seems :-)
func Test(c *gin.Context) {

	var apiError ErrorResponse

	/*
		apiError.Code = InvalidJSON
		apiError.Message = apiError.String(apiError.Code)
		c.JSON(http.StatusUnprocessableEntity, apiError)
		return
	*/

	/*c.Status(http.StatusInternalServerError)
	return*/

	// userID (currentUser) could be used to check a user's permission to view another profile
	/*
		userID, err := authentication.Authenticate(c.Request)
		if err != nil {
			c.JSON(http.StatusUnauthorized, err.Error())
			return
		}
	*/

	// fehlender parameter muss nicht geprüft werden, sonst wär's eine andere route
	user, err := environment.Env.UserModel.GetUserByID("5feb2473b4d37f7f0285847a")
	if err != nil {
		apiError.Code = InvalidRequest
		apiError.Message = apiError.String(apiError.Code)
		c.JSON(http.StatusUnprocessableEntity, apiError)
		return
	}

	// don't send password hash
	user.Password = ""

	c.JSON(http.StatusOK, &user)
}

// GetUser sends a profile
func GetUser(c *gin.Context) {

	/*
		var apiError ErrorResponse
		apiError.Code = InvalidJSON
		apiError.Message = apiError.String(apiError.Code)
		c.JSON(http.StatusUnprocessableEntity, apiError)
		return*
	*/
	/*
		c.Status(http.StatusInternalServerError)
		return
	*/

	// used to apply privacy rules if someone else is viewing the profile
	userID, err := authentication.Authenticate(c.Request)
	if err != nil {
		c.JSON(http.StatusUnauthorized, err.Error())
		return
	}

	// fehlender parameter muss nicht geprüft werden, sonst wär's eine andere route
	user, err := environment.Env.UserModel.GetUserByID(c.Param("id"))
	if err != nil {
		// nothing found (not an error to the client)
		if err == apperror.ErrNoData {
			c.Status(http.StatusNoContent)
			return
		}
		// technical errors
		status, apiError := HandleError(err)
		c.JSON(status, apiError)
		return
	}

	// apply privacy rules to visitors
	if userID != c.Param("id") {
		switch user.PrivacyCode {
		case lookups.PrivacyUserName:
			user.XBoxTag = ""
		case lookups.PrivacyXboxTag:
			user.LoginName = ""
		}
	}

	// don't send password hash
	user.Password = ""

	// build URL to profile picture
	// ToDo: via Helper in Model ?

	//		user.ProfilePictureURL = os.Getenv("API_HOME") + ":" + os.Getenv("API_PORT") + environment.UploadEndpoint + "/" + user.ProfilePictureData.SysFileName

	c.JSON(http.StatusOK, &user)

	// log this request, if it was a new one
	if environment.Env.Requests.Continue(getIP(c.Request), c.Param("id")) {
		environment.Env.Tracker.SaveVisitor("user", c.Param("id"), userID)
	}
}

// BlockUser adds someone to the user's ignorelist
func BlockUser(c *gin.Context) { // ToDo: Unlock

	var apiError ErrorResponse

	userID, err := authentication.Authenticate(c.Request)
	if err != nil {
		c.Status(http.StatusUnauthorized)
		return
	}

	// anonymous struct used to receive input (POST BODY)
	data := struct {
		BlockedUserID string `json:"blockedUserID" binding:"required"`
	}{}

	// use 'shouldBind' so we can send customized messages
	if err := c.ShouldBindJSON(&data); err != nil {
		apiError.Code = InvalidJSON
		apiError.Message = apiError.String(apiError.Code)
		c.JSON(http.StatusUnprocessableEntity, apiError)
		return
	}

	err = environment.Env.UserModel.BlockUser(userID, data.BlockedUserID)
	if err != nil {
		status, apiError := HandleError(err)
		c.JSON(status, apiError)
		return
	}
}

// UnblockUser removes someone from the user's ignorelist
func UnblockUser(c *gin.Context) { // ToDo: Unlock

	var apiError ErrorResponse

	userID, err := authentication.Authenticate(c.Request)
	if err != nil {
		c.Status(http.StatusUnauthorized)
		return
	}

	// anonymous struct used to receive input (POST BODY)
	data := struct {
		BlockedUserID string `json:"blockedUserID" binding:"required"`
	}{}

	// use 'shouldBind' so we can send customized messages
	if err := c.ShouldBindJSON(&data); err != nil {
		apiError.Code = InvalidJSON
		apiError.Message = apiError.String(apiError.Code)
		c.JSON(http.StatusUnprocessableEntity, apiError)
		return
	}

	err = environment.Env.UserModel.UnblockUser(userID, data.BlockedUserID)
	if err != nil {
		status, apiError := HandleError(err)
		c.JSON(status, apiError)
		return
	}
}

// GetFriends sends a profile
func GetFriends(c *gin.Context) {

	// userID (currentUser) could be used to check a user's permission to view another profile
	/*
		userID, err := authentication.Authenticate(c.Request)
		if err != nil {
			c.JSON(http.StatusUnauthorized, err.Error())
			return
		}
	*/

	// fehlender parameter muss nicht geprüft werden, sonst wär's eine andere route
	friends, err := environment.Env.UserModel.GetFriends(c.Param("id"))
	if err != nil {
		// nothing found (not an error to the client)
		if err == apperror.ErrNoData {
			c.Status(http.StatusNoContent)
			return
		}
		// technical errors
		status, apiError := HandleError(err)
		c.JSON(status, apiError)
		return
	}

	c.JSON(http.StatusOK, &friends)
}

// GetFollowings lists all people someone's following
func GetFollowings(c *gin.Context) {

	// userID (currentUser) could be used to check a user's permission to view another profile
	/*
		userID, err := authentication.Authenticate(c.Request)
		if err != nil {
			c.JSON(http.StatusUnauthorized, err.Error())
			return
		}
	*/

	// fehlender parameter muss nicht geprüft werden, sonst wär's eine andere route
	friends, err := environment.Env.UserModel.GetFollowings(c.Param("id"))
	if err != nil {
		// nothing found (not an error to the client)
		if err == apperror.ErrNoData {
			c.Status(http.StatusNoContent)
			return
		}
		// technical errors
		status, apiError := HandleError(err)
		c.JSON(status, apiError)
		return
	}

	c.JSON(http.StatusOK, &friends)
}

// GetFollowers lists all people who are following someone
func GetFollowers(c *gin.Context) {

	// userID (currentUser) could be used to check a user's permission to view another profile
	/*
		userID, err := authentication.Authenticate(c.Request)
		if err != nil {
			c.JSON(http.StatusUnauthorized, err.Error())
			return
		}
	*/

	// fehlender parameter muss nicht geprüft werden, sonst wär's eine andere route
	followers, err := environment.Env.UserModel.GetFollowers(c.Param("id"))
	if err != nil {
		// nothing found (not an error to the client)
		if err == apperror.ErrNoData {
			c.Status(http.StatusNoContent)
			return
		}
		// technical errors
		status, apiError := HandleError(err)
		c.JSON(status, apiError)
		return
	}

	c.JSON(http.StatusOK, &followers)
}

// AddFriend adds someone to the user's friendlist
func AddFriend(c *gin.Context) {

	var apiError ErrorResponse

	userID, err := authentication.Authenticate(c.Request)
	if err != nil {
		c.Status(http.StatusUnauthorized)
		return
	}

	// anonymous struct used to receive input (POST BODY)
	data := struct {
		FriendID string `json:"friendID" binding:"required"`
	}{}

	// use 'shouldBind' so we can send customized messages
	if err := c.ShouldBindJSON(&data); err != nil {
		apiError.Code = InvalidJSON
		apiError.Message = apiError.String(apiError.Code)
		c.JSON(http.StatusUnprocessableEntity, apiError)
		return
	}

	err = environment.Env.UserModel.AddFriend(userID, data.FriendID)
	if err != nil {
		status, apiError := HandleError(err)
		c.JSON(status, apiError)
		return
	}
}

// RemoveFriend adds someone to the user's friendlist
func RemoveFriend(c *gin.Context) {

	var apiError ErrorResponse

	userID, err := authentication.Authenticate(c.Request)
	if err != nil {
		c.Status(http.StatusUnauthorized)
		return
	}

	// ToDo: umstellwen auf query-param

	// anonymous struct used to receive input (POST BODY)
	// ToDo: mehrere auf einmal vorsehen - nötig?
	data := struct {
		FriendID string `json:"friendID" binding:"required"`
	}{}

	// use 'shouldBind' so we can send customized messages
	if err := c.ShouldBindJSON(&data); err != nil {
		apiError.Code = InvalidJSON
		apiError.Message = apiError.String(apiError.Code)
		c.JSON(http.StatusUnprocessableEntity, apiError)
		return
	}

	err = environment.Env.UserModel.RemoveFriend(userID, data.FriendID)
	if err != nil {
		status, apiError := HandleError(err)
		c.JSON(status, apiError)
		return
	}
}

// FollowUser adds someone to the user's friendlist
func FollowUser(c *gin.Context) {

	var apiError ErrorResponse

	userID, err := authentication.Authenticate(c.Request)
	if err != nil {
		c.Status(http.StatusUnauthorized)
		return
	}

	// anonymous struct used to receive input (POST BODY)
	// ToDo: mehrere auf einmal vorsehen - nötig?
	data := struct {
		UserID string `json:"userID" binding:"required"` // user to be followed
	}{}

	// use 'shouldBind' so we can send customized messages
	if err := c.ShouldBindJSON(&data); err != nil {
		apiError.Code = InvalidJSON
		apiError.Message = apiError.String(apiError.Code)
		c.JSON(http.StatusUnprocessableEntity, apiError)
		return
	}

	err = environment.Env.UserModel.FollowUser(userID, data.UserID)
	if err != nil {
		status, apiError := HandleError(err)
		c.JSON(status, apiError)
		return
	}
}

// UploadProfilePicture sets the profile picture
func UploadProfilePicture(c *gin.Context) {

	var (
		err error
		// data     models.UploadInfo
		apiError   ErrorResponse // declared here to raise own errors
		uploadInfo *models.UploadInfo
	)

	// (no post body available at forms)
	userID, err := authentication.Authenticate(c.Request)
	if err != nil {
		c.Status(http.StatusUnauthorized)
		return
	}

	// TODO: (in generic upload): Read Form Data - kein Body vorhanden!

	// https://github.com/gin-gonic/gin#single-file

	// single file
	file, err := c.FormFile("file")
	if err != nil {
		fmt.Println(err)
		apiError.Code = InvalidJSON
		apiError.Message = apiError.String(apiError.Code)
		c.JSON(http.StatusUnprocessableEntity, apiError)
		return
	}

	// https://www.devdungeon.com/content/working-files-go

	// ToDO: In Upload Model verschieben
	// generate file name & prepare metadata
	uploadInfo = new(models.UploadInfo)

	uploadInfo.ProfileID = helpers.ObjectID(userID)
	uploadInfo.ProfileType = "user"
	uploadInfo.ID = uuid.NewV4().String() // zufälliger dateiname (geht in stage fs/db)
	uploadInfo.UploadedID = uploadInfo.ProfileID
	// do not save path in the DB as this is subject to change
	// and don't use userID for file name
	uploadInfo.SysFileName = "usr_" + uploadInfo.ID + filepath.Ext(file.Filename)
	uploadInfo.OrigFileName = file.Filename
	// ToDO: Helpers oder helpers.file proc für path funcs (ohne punkt)
	// getStage, getTarget etc. die func arbeitet mit den envs
	// save File, returns uploadInfo
	uploadInfo.URL = os.Getenv("API_HOME") + ":" + os.Getenv("API_PORT") + environment.UploadEndpoint + "/" + uploadInfo.SysFileName

	stage := os.Getenv("UPLOAD_STAGE") + "/" + uploadInfo.SysFileName

	// Upload the file to specific stage
	err = c.SaveUploadedFile(file, stage)
	if err != nil {
		fmt.Println(err)
		apiError.Code = SystemError
		apiError.Message = apiError.String(apiError.Code)
		c.JSON(http.StatusInternalServerError, apiError)
		return
	}

	// move file to destination
	dst := os.Getenv("UPLOAD_TARGET") + "/" + uploadInfo.SysFileName
	err = os.Rename(stage, dst)
	if err != nil {
		fmt.Println(err)
		apiError.Code = SystemError
		apiError.Message = apiError.String(apiError.Code)
		c.JSON(http.StatusInternalServerError, apiError)
		return
	}

	// update meta data (registry)
	err = environment.Env.UserModel.SetProfilePicture(uploadInfo)
	if err != nil {
		fmt.Println(err)
		apiError.Code = SystemError
		apiError.Message = apiError.String(apiError.Code)
		c.JSON(http.StatusInternalServerError, apiError)
		return
	}

	c.JSON(http.StatusCreated, Uploaded{uploadInfo.URL})
}
