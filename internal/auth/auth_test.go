package auth

import "testing"

func TestPasswordHashing(t *testing.T) {
	tests := []struct {
		name      string
		password  string
		checkWith string
		wantMatch bool
	}{
		{
			name:      "correct password matches its hash",
			password:  "correct horse battery staple",
			checkWith: "correct horse battery staple",
			wantMatch: true,
		},
		{
			name:      "wrong password does not match",
			password:  "correct horse battery staple",
			checkWith: "Tr0ub4dor&3",
			wantMatch: false,
		},
		{
			name:      "empty password does not match a real hash",
			password:  "hunter2",
			checkWith: "",
			wantMatch: false,
		},
		{
			name:      "case matters",
			password:  "hunter2",
			checkWith: "Hunter2",
			wantMatch: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := HashPassword(tt.password)
			if err != nil {
				t.Fatalf("HashPassword(%q) returned error: %v", tt.password, err)
			}
			if hash == tt.password {
				t.Fatalf("HashPassword(%q) returned the plaintext password", tt.password)
			}
			err = CheckPasswordHash(tt.checkWith, hash)
			if tt.wantMatch && err != nil {
				t.Errorf("CheckPasswordHash(%q, hash of %q) = %v, want match", tt.checkWith, tt.password, err)
			}
			if !tt.wantMatch && err == nil {
				t.Errorf("CheckPasswordHash(%q, hash of %q) matched, want mismatch", tt.checkWith, tt.password)
			}
		})
	}
}

func TestCheckPasswordHashRejectsGarbageHash(t *testing.T) {
	if err := CheckPasswordHash("anything", "not-a-bcrypt-hash"); err == nil {
		t.Error("CheckPasswordHash against a non-bcrypt string matched, want error")
	}
}
