package main

import (
	"context"
	"encoding/json"
	"os"
	"strconv"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const counterItemID = "counter"

type counterPayload struct {
	Value int64 `json:"value"`
}

type counterRecord struct {
	ID    string `dynamodbav:"id"`
	Value int64  `dynamodbav:"value"`
}

func main() {
	tableName := os.Getenv("TABLE_NAME")
	if tableName == "" {
		panic("TABLE_NAME must be set")
	}

	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		panic(err)
	}

	db := dynamodb.NewFromConfig(cfg)

	lambda.Start(func(ctx context.Context, request events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
		switch request.RequestContext.HTTP.Method {
		case "GET":
			value, err := getCounterValue(ctx, db, tableName)
			if err != nil {
				return internalError(), nil
			}
			return htmlResponse(200, counterPayload{Value: value})
		case "POST":
			value, err := incrementCounterValue(ctx, db, tableName)
			if err != nil {
				return internalError(), nil
			}
			return jsonResponse(200, counterPayload{Value: value})
		default:
			return events.APIGatewayV2HTTPResponse{StatusCode: 405}, nil
		}
	})
}

func getCounterValue(ctx context.Context, db *dynamodb.Client, tableName string) (int64, error) {
	key, err := attributevalue.MarshalMap(map[string]string{"id": counterItemID})
	if err != nil {
		return 0, err
	}

	out, err := db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key:       key,
	})
	if err != nil {
		return 0, err
	}

	if len(out.Item) == 0 {
		return 0, nil
	}

	var item counterRecord
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return 0, err
	}

	return item.Value, nil
}

func incrementCounterValue(ctx context.Context, db *dynamodb.Client, tableName string) (int64, error) {
	out, err := db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: counterItemID},
		},
		ExpressionAttributeNames: map[string]string{
			"#value": "value",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":zero": &types.AttributeValueMemberN{Value: "0"},
			":inc":  &types.AttributeValueMemberN{Value: "1"},
		},
		UpdateExpression: aws.String("SET #value = if_not_exists(#value, :zero) + :inc"),
		ReturnValues:     types.ReturnValueUpdatedNew,
	})
	if err != nil {
		return 0, err
	}

	if len(out.Attributes) == 0 {
		return 0, nil
	}

	var item counterRecord
	if err := attributevalue.UnmarshalMap(out.Attributes, &item); err != nil {
		return 0, err
	}

	return item.Value, nil
}

func jsonResponse(statusCode int, payload counterPayload) (events.APIGatewayV2HTTPResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return internalError(), nil
	}

	return events.APIGatewayV2HTTPResponse{
		StatusCode: statusCode,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

func htmlResponse(statusCode int, payload counterPayload) (events.APIGatewayV2HTTPResponse, error) {
	valueText := strconv.FormatInt(payload.Value, 10)
	body := `
		<!DOCTYPE html>
		<html>
		<head>
			<title>Counter</title>
		</head>
		<body>
			<h1>Counter Value: ` + valueText + `</h1>
			<form method="post">
				<button type="submit">Increment</button>
			</form>
		</body>
		<script>
			// post increment request using fetch API
			const form = document.querySelector('form');
			form.addEventListener('submit', async (e) => {
				e.preventDefault();
				const response = await fetch('/', { method: 'POST' });
				const data = await response.json();
				document.querySelector('h1').textContent = 'Counter Value: ' + data.value;
			});
		</script>
		</html>
	`
	return events.APIGatewayV2HTTPResponse{
		StatusCode: statusCode,
		Headers: map[string]string{
			"Content-Type": "text/html",
		},
		Body: body,
	}, nil
}

func internalError() events.APIGatewayV2HTTPResponse {
	return events.APIGatewayV2HTTPResponse{
		StatusCode: 500,
		Body:       "internal server error",
	}
}
