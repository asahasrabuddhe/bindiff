// Package selfupdate updates a running program in place by applying a
// bindiff patch to its own executable.
//
// A release publishes a manifest, bindiff-manifest.json, next to its
// patches. Each manifest entry names a patch and the SHA-256 of the binary
// it applies to and the binary it produces. An Updater hashes the running
// executable, finds the entry for that exact binary, downloads the patch,
// checks every hash, and swaps the new binary into place.
//
// Because entries are matched by hash, a patch is never applied to the wrong
// bytes. Version strings are informational only.
//
// Manifests can be signed with ed25519. Set Updater.PublicKey to require a
// valid signature. The bindiff command's keygen subcommand makes a key pair,
// and its release subcommand writes and signs manifests.
//
// A minimal update command:
//
//	u := &selfupdate.Updater{
//		Source:    selfupdate.GitHub("owner", "repo"),
//		PublicKey: publicKey,
//	}
//	update, err := u.Check(ctx)
//	switch {
//	case errors.Is(err, selfupdate.ErrUpToDate):
//		return nil
//	case errors.Is(err, selfupdate.ErrNoPatch):
//		// No patch from this build. Fall back to a full download.
//	case err != nil:
//		return err
//	}
//	return u.Apply(ctx, update)
//
// Call CleanupOld early in main. On Windows a running executable cannot be
// deleted, so Apply leaves the previous binary next to the new one with an
// .old suffix, and CleanupOld removes it on the next start.
package selfupdate
