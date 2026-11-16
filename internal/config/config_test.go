package config

import (
	"testing"

	"go.mongodb.org/mongo-driver/x/mongo/driver/connstring"
)

func TestURI(t *testing.T) {
	cases := []struct {
		name string
		db   DatabaseConfig
		want string
	}{
		{
			"sem credenciais",
			DatabaseConfig{Driver: "mongodb", Host: "localhost", Port: "27017", Params: "directConnection=true"},
			"mongodb://localhost:27017/?directConnection=true",
		},
		{
			"srv ignora a porta",
			DatabaseConfig{Driver: "mongodb+srv", Host: "cluster0.exemplo.net", Port: "27017", User: "nexus", Password: "segredo"},
			"mongodb+srv://nexus:segredo@cluster0.exemplo.net/",
		},
		{
			"senha com caracteres reservados",
			DatabaseConfig{Driver: "mongodb", Host: "mongo.railway.internal", Port: "27017", User: "mongo", Password: "p@ss:/w?#%", Params: "authSource=admin"},
			"mongodb://mongo:p%40ss%3A%2Fw%3F%23%25@mongo.railway.internal:27017/?authSource=admin",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.db.URI(); got != tc.want {
				t.Errorf("URI() = %q, quer %q", got, tc.want)
			}
		})
	}
}

func TestURIKeepsCredentials(t *testing.T) {
	db := DatabaseConfig{Driver: "mongodb", Host: "mongo.railway.internal", Port: "27017", User: "us@r", Password: "p@ss:/w?#%"}
	cs, err := connstring.Parse(db.URI())
	if err != nil {
		t.Fatal(err)
	}
	if cs.Username != db.User || cs.Password != db.Password {
		t.Errorf("driver leu %q / %q", cs.Username, cs.Password)
	}
}
