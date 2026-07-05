package warehouse

import "testing"

func TestResolveEnvDataset(t *testing.T) {
	// BIGQUERY_DATASET fully-qualified as "project.dataset" (the form used by
	// .env.example and the Cloud Run deploy docs) must be split, not used verbatim.
	ref, err := resolveEnvDataset("saanay-genai-adk", "saanay-genai-adk.retail")
	if err != nil {
		t.Fatalf("qualified: unexpected error %v", err)
	}
	if ref.Project != "saanay-genai-adk" || ref.Dataset != "retail" {
		t.Errorf("qualified: got %+v, want project=saanay-genai-adk dataset=retail", ref)
	}

	// A bare dataset name uses GCLOUD_PROJECT.
	ref, err = resolveEnvDataset("myproj", "retail")
	if err != nil {
		t.Fatalf("bare: unexpected error %v", err)
	}
	if ref.Project != "myproj" || ref.Dataset != "retail" {
		t.Errorf("bare: got %+v, want project=myproj dataset=retail", ref)
	}

	// Missing values error.
	if _, err := resolveEnvDataset("", "retail"); err == nil {
		t.Error("expected error when project is empty")
	}
	if _, err := resolveEnvDataset("myproj", ""); err == nil {
		t.Error("expected error when dataset is empty")
	}
}
