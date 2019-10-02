package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	pb "github.com/hyperledger/fabric/protos/peer"
)

type SimpleChaincode struct {
}

type FileDetails struct {
	FileName string
	FileHash string
	FileUrl  string
}

// Init is called during chaincode instantiation to initialize any
// data. Note that chaincode upgrade also calls this function to reset
// or to migrate data.
func (t *SimpleChaincode) Init(stub shim.ChaincodeStubInterface) pb.Response {
	return shim.Success(nil)
}

func (t *SimpleChaincode) Invoke(stub shim.ChaincodeStubInterface) pb.Response {
	function, args := stub.GetFunctionAndParameters()
	fmt.Println("invoke is running " + function)
	// Handle different functions
	if function == "initFile" { //create a new marble
		return t.initFile(stub, args)
	} else if function == "deletefile" { //delete a marble
		return t.deleteFile(stub, args)
	} else if function == "queryfile" { //find marbles based on an ad hoc rich query
		return t.queryfile(stub, args)
	}
	fmt.Println("invoke did not find func: " + function) //error
	return shim.Error("Received unknown function invocation")
}

// ============================================================
// initFile - create a new marble, store into chaincode state
// ============================================================
func (t *SimpleChaincode) initFile(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	//   0       		1     		  2
	// "filename", "filehash", "fileurl",
	if len(args) != 3 {
		return shim.Error("Incorrect number of arguments. Expecting 4")
	}
	// ==== Input sanitation ====
	fmt.Println("- start init file")
	if len(args[0]) <= 0 {
		return shim.Error("1st argument must be a non-empty string")
	}
	if len(args[1]) <= 0 {
		return shim.Error("2nd argument must be a non-empty string")
	}
	if len(args[2]) <= 0 {
		return shim.Error("3rd argument must be a non-empty string")
	}

	fileName := args[0]
	fileHash := strings.ToLower(args[1])
	fileUrl := strings.ToLower(args[3])

	// ==== Check if file name  already exists ====
	filenameAsBytes, err := stub.GetState(fileName)
	if err != nil {
		return shim.Error("Failed to get file: " + err.Error())
	} else if filenameAsBytes != nil {
		fmt.Println("This filename already exists: " + fileName)
		return shim.Error("This file is  already exists: " + fileName)
	}
	// ==== Create file object and marshal to JSON ====
	filestore := &FileDetails{fileName, fileHash, fileUrl}
	fileJSONasBytes, err := json.Marshal(filestore)
	if err != nil {
		return shim.Error(err.Error())
	}

	// === Save file hash to state ===
	err = stub.PutState(fileName, fileJSONasBytes)
	if err != nil {
		return shim.Error(err.Error())
	}

	indexName := "hash~name"
	fileHashNameIndexKey, err := stub.CreateCompositeKey(indexName, []string{filestore.FileHash, filestore.FileName})
	if err != nil {
		return shim.Error(err.Error())
	}
	//  Save index entry to state. Only the key name is needed, no need to store a duplicate copy of the marble.
	//  Note - passing a 'nil' value will effectively delete the key from state, therefore we pass null character as value
	value := []byte{0x00}
	stub.PutState(fileHashNameIndexKey, value)

	// ==== file saved and indexed. Return success ====
	fmt.Println("- end init filestored")
	return shim.Success(nil)

}

// ==================================================
// delete - remove a marble key/value pair from state
// ==================================================
func (t *SimpleChaincode) deleteFile(stub shim.ChaincodeStubInterface, args []string) pb.Response {
	var jsonResp string
	var fileJSON FileDetails
	if len(args) != 1 {
		return shim.Error("Incorrect number of arguments. Expecting 1")
	}
	filename := args[0]

	// to maintain the color~name index, we need to read the marble first and get its color
	valAsbytes, err := stub.GetState(filename) //get the marble from chaincode state
	if err != nil {
		jsonResp = "{\"Error\":\"Failed to get state for " + filename + "\"}"
		return shim.Error(jsonResp)
	} else if valAsbytes == nil {
		jsonResp = "{\"Error\":\"Marble does not exist: " + filename + "\"}"
		return shim.Error(jsonResp)
	}

	err = json.Unmarshal([]byte(valAsbytes), &fileJSON)
	if err != nil {
		jsonResp = "{\"Error\":\"Failed to decode JSON of: " + filename + "\"}"
		return shim.Error(jsonResp)
	}

	err = stub.DelState(filename) //remove the marble from chaincode state
	if err != nil {
		return shim.Error("Failed to delete state:" + err.Error())
	}

	// maintain the index
	indexName := "Hash~name"
	colorNameIndexKey, err := stub.CreateCompositeKey(indexName, []string{fileJSON.FileHash, fileJSON.FileName})
	if err != nil {
		return shim.Error(err.Error())
	}

	//  Delete index entry to state.
	err = stub.DelState(colorNameIndexKey)
	if err != nil {
		return shim.Error("Failed to delete state:" + err.Error())
	}
	return shim.Success(nil)
}

// ===== Example: Ad hoc rich query ========================================================
// queryMarbles uses a query string to perform a query for marbles.
// Query string matching state database syntax is passed in and executed as is.
// Supports ad hoc queries that can be defined at runtime by the client.
// If this is not desired, follow the queryMarblesForOwner example for parameterized queries.
// Only available on state databases that support rich query (e.g. CouchDB)
// =========================================================================================
func (t *SimpleChaincode) queryfile(stub shim.ChaincodeStubInterface, args []string) pb.Response {

	//   0
	// "queryString"
	if len(args) < 1 {
		return shim.Error("Incorrect number of arguments. Expecting 1")
	}

	queryString := args[0]

	queryResults, err := getQueryResultForQueryString(stub, queryString)
	if err != nil {
		return shim.Error(err.Error())
	}
	return shim.Success(queryResults)
}

//start the chain code
func main() {
	err := shim.Start(new(SimpleChaincode))
	if err != nil {
		fmt.Printf("Error starting Simple chaincode: %s", err)
	}
}

// =========================================================================================
// getQueryResultForQueryString executes the passed in query string.
// Result set is built and returned as a byte array containing the JSON results.
// =========================================================================================
func getQueryResultForQueryString(stub shim.ChaincodeStubInterface, queryString string) ([]byte, error) {

	fmt.Printf("- getQueryResultForQueryString queryString:\n%s\n", queryString)

	resultsIterator, err := stub.GetQueryResult(queryString)
	if err != nil {
		return nil, err
	}
	defer resultsIterator.Close()

	buffer, err := constructQueryResponseFromIterator(resultsIterator)
	if err != nil {
		return nil, err
	}

	fmt.Printf("- getQueryResultForQueryString queryResult:\n%s\n", buffer.String())

	return buffer.Bytes(), nil
}

// ===========================================================================================
// constructQueryResponseFromIterator constructs a JSON array containing query results from
// a given result iterator
// ===========================================================================================
func constructQueryResponseFromIterator(resultsIterator shim.StateQueryIteratorInterface) (*bytes.Buffer, error) {
	// buffer is a JSON array containing QueryResults
	var buffer bytes.Buffer
	buffer.WriteString("[")

	bArrayMemberAlreadyWritten := false
	for resultsIterator.HasNext() {
		queryResponse, err := resultsIterator.Next()
		if err != nil {
			return nil, err
		}
		// Add a comma before array members, suppress it for the first array member
		if bArrayMemberAlreadyWritten == true {
			buffer.WriteString(",")
		}
		buffer.WriteString("{\"Key\":")
		buffer.WriteString("\"")
		buffer.WriteString(queryResponse.Key)
		buffer.WriteString("\"")

		buffer.WriteString(", \"Record\":")
		// Record is a JSON object, so we write as-is
		buffer.WriteString(string(queryResponse.Value))
		buffer.WriteString("}")
		bArrayMemberAlreadyWritten = true
	}
	buffer.WriteString("]")

	return &buffer, nil
}
