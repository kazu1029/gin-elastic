package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/olivere/elastic"
	"github.com/teris-io/shortid"
)

const (
	elasticIndexName = "documents"
	elasticTypeName  = "document"
)

type Document struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Content   string    `json:"content"`
}

type DocumentRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

type DocumentResponse struct {
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Content   string    `json:"content"`
}

type SearchResponse struct {
	Time      string             `json:"time"`
	Hits      string             `json:"hits"`
	Documents []DocumentResponse `json:"documents"`
}

// FIXME: Need to change mapping
const mapping = `
{
	"settings": {
	  "analysis": {
			"analyzer": {
			  "ja_kuromoji_analyzer": {
					"type": "custom",
					"tokenizer": "kuromoji_tokenizer",
					"filter": [
					  "kuromoji_baseform",
						"kuromoji_part_of_speech",
						"kuromoji_readingform"
					]
				}
			}
		}
	},
	"mappings": {
		"document":{
			"properties":{
				"content": {
					"type": "text",
					"analyzer": "ja_kuromoji_analyzer"
				},
				"title": {
					"type": "text",
					"analyzer": "ja_kuromoji_analyzer"
				}
			}
		}
	}
}
`

func CreateDocumentsEndpoint(c *gin.Context) {
	elasticClient, err := InitElastic()
	if err != nil {
		log.Println(err)
	}
	var docs []DocumentRequest
	// TODO: unable to bind japanese
	if err := c.BindJSON(&docs); err != nil {
		fmt.Printf("err is %+v\n", err)
		errorResponse(c, http.StatusBadRequest, "Malformed request body")
		return
	}

	bulk := elasticClient.
		Bulk().
		Index(elasticIndexName).
		Type(elasticTypeName)

	for _, d := range docs {
		doc := Document{
			ID:        shortid.MustGenerate(),
			Title:     d.Title,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
			Content:   d.Content,
		}
		bulk.Add(elastic.NewBulkIndexRequest().Id(doc.ID).Doc(doc))
	}
	if _, err := bulk.Do(c.Request.Context()); err != nil {
		log.Printf("err is %+v\n", err)
		errorResponse(c, http.StatusInternalServerError, "Failed to create documents")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Document Created",
	})
}

func CreateMapping(c *gin.Context) {
	elasticClient, err := InitElastic()
	ctx := context.Background()
	if err != nil {
		log.Println(err)
	}
	exists, err := elasticClient.IndexExists(elasticIndexName).Do(ctx)
	if err != nil {
		log.Println(err)
	}

	if !exists {
		createIndex, err := elasticClient.CreateIndex(elasticIndexName).BodyString(mapping).Do(ctx)
		if err != nil {
			log.Println(err)
		}
		if !createIndex.Acknowledged {
			log.Println(createIndex)
		} else {
			log.Println("successfully created index")
		}
	} else {
		log.Println("Index already exists")
	}
}

func SearchEndpoint(c *gin.Context) {
	elasticClient, err := InitElastic()
	queries := c.Request.URL.Query()
	query := queries["query"][0]
	if query == "" {
		errorResponse(c, http.StatusBadRequest, "Query not specified")
		return
	}

	skip := 0
	take := 10
	fmt.Printf("skip is %+v\n", queries["skip"])
	if i, err := strconv.Atoi(c.Query("skip")); err == nil {
		skip = i
	}
	if i, err := strconv.Atoi(c.Query("take")); err == nil {
		take = i
	}

	esQuery := elastic.NewMultiMatchQuery(query, "title", "content").
		Fuzziness("2").
		MinimumShouldMatch("2")
	result, err := elasticClient.Search().
		Index(elasticIndexName).
		Query(esQuery).
		From(skip).Size(take).
		Do(c.Request.Context())
	if err != nil {
		log.Printf("err is %v\n", err)
		errorResponse(c, http.StatusInternalServerError, "Something went wrong")
		return
	}
	res := SearchResponse{
		Time: fmt.Sprintf("%d", result.TookInMillis),
		Hits: fmt.Sprintf("%d", result.Hits.TotalHits),
	}
	docs := make([]DocumentResponse, 0)
	for _, hit := range result.Hits.Hits {
		var doc DocumentResponse
		json.Unmarshal(*hit.Source, &doc)
		docs = append(docs, doc)
	}
	res.Documents = docs

	c.JSON(http.StatusOK, res)
}
