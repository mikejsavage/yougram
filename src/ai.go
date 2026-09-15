package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	// "mikegram/facedetect"
)

type AIModel struct {
	Name string
	Sha256 string
}

var facial_recognition_models = []AIModel {
	AIModel { "antelopev2.gguf", "245e657e51754fbf075dd43d80a80a2d14a60c2fc42a3220f63eef17a315e96c" }, // https://huggingface.co/mudler/face-detect-gguf/blob/main/antelopev2.gguf
	AIModel { "buffalo_l.gguf",  "6ed070f6e569beeed542ddd5603bcbc9eb8ea57f728f7d8013d6a90b2b952116" }, // https://huggingface.co/mudler/face-detect-gguf/blob/main/buffalo_l.gguf
	AIModel { "buffalo_m.gguf",  "0f7527eeb97b88719bf7e11e43ab8af6f05999357d767f8dde53db3c586c1c3f" }, // https://huggingface.co/mudler/face-detect-gguf/blob/main/buffalo_m.gguf
	AIModel { "buffalo_s.gguf",  "7490b1efbc8746b188a5aef0adf5e3d1a2dc9607abd474018893f95571999969" }, // https://huggingface.co/mudler/face-detect-gguf/blob/main/buffalo_s.gguf
}

func sha256File( path string ) ( []byte, error ) {
	f, err := os.Open( path )
	if err != nil {
		return nil, err
	}
	defer f.Close()

	hasher := sha256.New()
	_, err = io.Copy( hasher, f )
	if err != nil {
		return nil, err
	}

	return hasher.Sum( nil ), nil
}

var facial_clustering_ticker *time.Ticker

func initFacialRecognition() {
	files, err := os.ReadDir( "ai" )
	if errors.Is( err, os.ErrNotExist ) {
		return
	} else {
		must( err )
	}

	paths := make( []string, len( facial_recognition_models ) )
    for _, f := range files {
		sha256 := hex.EncodeToString( must1( sha256File( "ai/" + f.Name() ) ) )
		recognised := false
		for i, model := range facial_recognition_models {
			if sha256 == model.Sha256 {
				if paths[ i ] == "" {
					paths[ i ] = f.Name()
				} else {
					fmt.Printf( "You have the same model twice: %s and %s\n", f.Name(), paths[ i ] )
				}
				recognised = true
			}
		}

		if !recognised {
			fmt.Printf( "Unrecognised model: ai/%s\n", f.Name() )
		}
    }

	best_model := len( facial_recognition_models )
	for i, f := range paths {
		if f != "" {
			best_model = i
			break
		}
	}

	if best_model == len( facial_recognition_models ) {
		return
	}

	fmt.Printf( "Using %s (ai/%s) for facial recognition\n", facial_recognition_models[ best_model ].Name, paths[ best_model ] )

	facial_clustering_ticker = time.NewTicker( sel( IsReleaseBuild, time.Minute * 10, time.Second ) )

	go func() {
		for {
			<- facial_clustering_ticker.C
			// TODO: make a context for this so we can cancel it for fast shutdown
			if must1( queries.NeedToClusterFaces( context.Background() ) ).( int64 ) == 1 {
				fmt.Printf( "cluster time\n" )
				must( queries.DidClusterFaces( context.Background() ) )
			}
		}
		fmt.Printf( "cya\n" ) // TODO: should probably make this shut down cleanly since it hits the DB
	}()
}

func shutdownFacialRecognition() {
	facial_clustering_ticker.Stop()
}
