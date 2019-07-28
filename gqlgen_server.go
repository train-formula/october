package october

import (
	"fmt"
	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/handler"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"time"
)

type GQLGenServer struct {

	mode Mode

	address string
	port    int

	schema graphql.ExecutableSchema
	options []handler.Option
}

func (g *GQLGenServer) playgroundHandler() gin.HandlerFunc {
	h := handler.Playground("GraphQL", "/query")

	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}

func (g *GQLGenServer) graphqlHandler() gin.HandlerFunc {
	h := handler.GraphQL(g.schema, g.options...)

	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}

func (g *GQLGenServer) WithExecutableSchema(schema graphql.ExecutableSchema) {
	g.schema = schema
}

func (g *GQLGenServer) WithOptions(options ...handler.Option) {
	g.options = options
}


func (g *GQLGenServer) Start() error {
	if g.schema == nil {
		zap.L().Named("OCTOBER").Fatal("Missing gqlgen executable schema, call WithExecutableSchema before Start ")
	}

	server := gin.New()

	server.Use(Ginzap(zap.L(), time.RFC3339, true), RecoveryWithZap(zap.L(), true))

	if g.mode == LOCAL {
		server.GET("/", g.playgroundHandler())
	}
	server.POST("/query", g.graphqlHandler())

	return server.Run(fmt.Sprintf("%s:%d", g.address, g.port))
}