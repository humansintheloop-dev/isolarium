package ec2

// RemoteUser is the login account on the Ubuntu AMI every instance boots from.
const RemoteUser = "ubuntu"

// RemoteRepoDir is where create places the repository checkout, matching the
// Lima convention of <home>/repo. Every command an isolated session runs starts
// from here.
const RemoteRepoDir = "/home/" + RemoteUser + "/repo"
