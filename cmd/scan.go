package cmd

import (
	"fmt"
	"github.com/bthuilot/git-scanner/pkg/git"
	"github.com/bthuilot/git-scanner/pkg/scanning"
	gogit "github.com/go-git/go-git/v5"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"os"
	"strings"
)

var (
	// Used for flags.
	repoURL       string
	repoPath      string
	outputPath    string
	scanner       string
	scannerConfig string
	userArgs      string
	keepRefs      bool
)

func init() {
	scanCmd.PersistentFlags().StringVarP(&repoURL, "repo-url", "r", "", "URL of the git repository to scan")
	scanCmd.PersistentFlags().StringVarP(&repoPath, "repo-path", "p", "", "Path to the git repository to scan")
	scanCmd.PersistentFlags().StringVarP(&outputPath, "output", "o", "", "Path to the output directory")
	scanCmd.PersistentFlags().StringVarP(&scanner, "scanner", "s", "gitleaks", "Scanner to use (gitleaks, trufflehog)")
	scanCmd.PersistentFlags().StringVarP(&scannerConfig, "scanner-config", "c", "", "Path to the scanner config file")
	scanCmd.PersistentFlags().StringVar(&userArgs, "scanner-args", "", "additional arguments to pass to the scanner")
	scanCmd.PersistentFlags().BoolVarP(&keepRefs, "keep-refs", "k", false, "Keep refs created for dangling commits")
	_ = scanCmd.MarkPersistentFlagFilename("repo-path")
	_ = scanCmd.MarkPersistentFlagFilename("scanner-config")
	scanCmd.MarkFlagsMutuallyExclusive("repo-url", "repo-path")
	scanCmd.MarkFlagsOneRequired("repo-url", "repo-path")
	rootCmd.AddCommand(scanCmd)
}

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "scan all commits of a git repository",
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		var output = os.Stdout
		if outputPath != "" && outputPath != "-" {
			output, err = os.Create(outputPath)
			if err != nil {
				return err
			}
			defer output.Close()
		}

		// clone repo or import existing repo
		r, dir, err := getGitRepository()
		if err != nil {
			return err
		}

		logrus.Debugf("Cloned or imported repository '%s'", dir)

		danglingObjs, err := git.FindDanglingObjects(r, dir)
		if err != nil {
			return err
		}

		// TODO: additional support scanning for blobs
		var createdRefs []string
		logrus.Infof("Found %d dangling commits", len(danglingObjs.Commits))
		for _, c := range danglingObjs.Commits {
			logrus.Debugf("Dangling commit: %s", c.Hash.String())
			ref := fmt.Sprintf("refs/dangling/%s", c.Hash.String())
			if err = git.MakeRef(r, ref, c); err != nil {
				logrus.Warnf("Failed to create ref for dangling commit: %s", c.Hash.String())
				continue
			}
			createdRefs = append(createdRefs, ref)
		}

		logrus.Infof("Created %d refs for dangling commits", len(createdRefs))
		if !keepRefs {
			defer func() {
				if err = git.RemoveReferences(r, createdRefs); err != nil {
					logrus.Errorf("Failed to remove created refs: %s", err)
				}
			}()
		}

		scannerArgs := strings.Split(userArgs, " ")
		if scannerConfig != "" {
			scannerArgs = append(scannerArgs, fmt.Sprintf("--config=%s", scannerConfig))
		}

		switch strings.ToLower(scanner) {
		case "gitleaks":
			err = scanning.RunGitleaks(dir, outputPath, scannerArgs...)
		case "trufflehog":
			err = scanning.RunTrufflehog(dir, outputPath, scannerArgs...)
		default:
			err = fmt.Errorf("unknown scanner '%s'", scanner)
		}

		return err
	},
}

func getGitRepository() (*gogit.Repository, string, error) {
	var (
		r   *gogit.Repository
		dir string = repoPath
		err error
	)
	if repoURL != "" {
		r, dir, err = git.CloneRepo(repoURL)
		if err != nil {
			return nil, "", err
		}
		logrus.Infof("Cloned repo: %s", repoURL)
	} else {
		r, err = git.ExistingRepo(repoPath)
		if err != nil {
			return nil, "", err
		}
		logrus.Infof("Using existing repo: %s", repoPath)
	}
	return r, dir, nil
}
