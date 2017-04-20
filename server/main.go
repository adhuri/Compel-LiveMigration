package main

import (
	"encoding/gob"
	"flag"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"

	migrationProtocol "github.com/adhuri/Compel-Migration/protocol"
	model "github.com/adhuri/Compel-Migration/server/model"
	strategy "github.com/adhuri/Compel-Migration/server/strategy"
)

var (
	log    *logrus.Logger
	server *model.Server
)

func init() {

	log = logrus.New()

	// Output logging to stdout
	log.Out = os.Stdout

	// Only log the info severity or above.
	log.Level = logrus.DebugLevel
	// Microseconds level logging
	customFormatter := new(logrus.TextFormatter)
	customFormatter.TimestampFormat = "2006-01-02 15:04:05.000000"
	customFormatter.FullTimestamp = true

	log.Formatter = customFormatter

}

func main() {
	// tcp listener

	immovableContainers := flag.String("immovable", "", "Comma seperated container IDs of immovable containers eg \"mysql1,mysql2,ff7a945953c7\" ")
	migrationFeatureStatus := flag.Bool("migrate", false, "true to turn on Migration Feature ")

	flag.Parse()

	log.WithFields(logrus.Fields{
		"immovable": *immovableContainers,
		"migrate":   *migrationFeatureStatus,
	}).Infoln("Inputs from command line")

	immovableContainersList := strings.Split(*immovableContainers, ",")
	log.Infoln("Immovable Containers List ", immovableContainersList)

	server = model.NewServer(immovableContainersList)

	// Testing thrashing
	//server.SetPreviousContainerMigrationTime("container41", time.Now().Unix())
	// End

	var wg sync.WaitGroup
	wg.Add(1)

	go tcpListener(&wg, server, *migrationFeatureStatus)

	wg.Wait()

}

func handlePredictionDataMessage(conn net.Conn, server *model.Server) {
	// Read the ConnectRequest
	predictionDataMessage := migrationProtocol.PredictionData{}
	decoder := gob.NewDecoder(conn)
	err := decoder.Decode(&predictionDataMessage)
	//err := binary.Read(conn, binary.LittleEndian, &connectMessage)
	if err != nil {
		// If failure in parsing, close the connection and return
		log.Errorln("Bad Prediction Data Message From Client" + err.Error())
		return
	} else {
		// If success, print the message received
		log.Infoln("Prediction Data Received")
		log.Debugln("Prediction Data Content : ", predictionDataMessage)
	}

	// Create a ConnectAck Message
	predictionAck := migrationProtocol.NewPredictionDataResponse(predictionDataMessage.Timestamp, true)

	// Send Connect Ack back to the client
	encoder := gob.NewEncoder(conn)
	err = encoder.Encode(predictionAck)
	//err = binary.Write(conn, binary.LittleEndian, connectAck)
	if err != nil {
		// If failure in parsing, close the connection and return
		log.Errorln("Prediction Data Ack")
		return
	}

	//fmt.Println("Yayy ----------------------", predictionDataMessage)
	ts := strconv.FormatInt(predictionDataMessage.Timestamp, 10)
	log.Infoln("Prediction Data Ack Sent for Request Id " + ts)
	// close connection when done
	conn.Close()

	// migration decision
	migrationNeeded, migrationInfo := strategy.MigrationNeeded(&predictionDataMessage, server, log)

	// send migration request if decided to migrate
	if migrationNeeded {
		err = SendMigrationRequest(migrationInfo, server, log)
		if err != nil {
			log.Infoln("Migration Was Failure")
			return
		}
		log.Infoln("Migration Was Success")
		// Log previous migration time if successful
		server.SetPreviousContainerMigrationTime(migrationInfo.ContainerID, time.Now().Unix())

	} else {
		log.Infoln("Migration Was Not Needed")
	}
}

func tcpListener(wg *sync.WaitGroup, server *model.Server, migrationFeatureStatus bool) {
	defer wg.Done()
	// Server listens on all interfaces for TCP connestion
	addr := ":" + "5051"
	log.Infoln("Migration Server listening on TCP ", addr)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalln("Server Failed To Start ")
	}

	// Wait for clients to connect
	for {
		// Accept a connection and spin-off a goroutine
		conn, err := listener.Accept()
		if err != nil {
			// If error continue to wait for other clients to connect
			continue
		}
		log.Infoln("Accepted Connection from Prediction Client ")
		if migrationFeatureStatus {
			go handlePredictionDataMessage(conn, server)
		}
		// If migration Feature is disabled - Accept Connection log Warn that Migration is not enabled
		// Migration Feature exists here to avoid prediction client failing
		log.Warnln("Migration Feature is disabled: enable using migrate=true")

	}
}
