package main

import (
	"Notifier/internal/notifier"
	"Notifier/internal/utils"
	"context"
	firebase "firebase.google.com/go"
	"fmt"
	"google.golang.org/api/option"
	"log"
	"os"
	"time"
)

func main() {
	start := time.Now()

	boxCountMaxNumLog, err := os.OpenFile("logs/boxCountMaxNumLog.txt", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal(err)
	}
	defer boxCountMaxNumLog.Close()

	errorLog, err := os.OpenFile("logs/errorLog.txt", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal(err)
	}
	defer errorLog.Close()

	sentNoticeLog, err := os.OpenFile("logs/sentNoticeLog.txt", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal(err)
	}
	defer sentNoticeLog.Close()

	utils.BoxCountMaxNumLogger = log.New(boxCountMaxNumLog, "", log.Ldate|log.Ltime|log.Lshortfile)
	utils.ErrorLogger = log.New(errorLog, "", log.Ldate|log.Ltime|log.Lshortfile)
	utils.SentNoticeLogger = log.New(sentNoticeLog, "", log.Ldate|log.Ltime|log.Lshortfile)

	ctx := context.Background()
	sa := option.WithCredentialsFile("serviceAccountKey.json")
	app, err := firebase.NewApp(ctx, nil, sa)
	if err != nil {
		utils.ErrorLogger.Fatal(err)
	}
	utils.Client, err = app.Firestore(ctx)
	if err != nil {
		utils.ErrorLogger.Fatal(err)
	}
	defer utils.Client.Close()

	notifier.AjouNormalBoxCount, notifier.AjouNormalMaxNum = notifier.GetAjouNormalFromDB()

	notifier.AjouNormalFunc(notifier.AjouNormalBoxCount, notifier.AjouNormalMaxNum)

	end := time.Since(start)
	fmt.Println("end =>", end)
}
