package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

//parseAndRun  解析命令行指令，并运行
func parseAndRun(cmd string) {
	var e error
	cmd, e = getSafeCmd(cmd)
	cmd = decodeAliasCmd(cmd)
	if e != nil {
		logger.Print(e)
		return
	}
	concurrent = make(chan int, cfg.Forks)
	brd := bufio.NewReader(os.Stdin)
	for {
		str, e := brd.ReadString('\n')
		if e != nil {
			if e == io.EOF {
				break
			} else {
				logger.Fatal(e)
			}
		}
		originIP := strings.Trim(str, "\n  \t")
		ipList := splitIP(originIP)
		for _, ip := range ipList {
			if isIP(ip) {
				concurrent <- 1
				wg.Add(1)
				if cfg.RemoteRun {
					go copyAndRun(ip, cmd)
				} else if cfg.Copy {
					go copyOnly(ip, cmd)
				} else {
					go run(ip, cmd)
				}
			} else {
				logger.Println(ip, "is not ip")
			}
		}
	}
	wg.Wait()
}

//getSafeCmd 处理一些简单的危险命令
func getSafeCmd(cmd string) (newCmd string, err error) {
	err = nil
	newCmd = strings.Trim(cmd, " \n\t")
	if cmd == "/" {
		err = errors.New("cmd can not be '/'")
		return
	}
	strSlice := strings.Fields(cmd)
	if strSlice[0] == "rm" {
		for _, str := range strSlice {
			if str == "/" || str == "/*" {
				err = errors.New("[danger] cmd can not  be 'rm /' or 'rm /*'")
				return
			}
		}
	}
	return
}

//decodeAliasCmd 处理、转换命令别名，别名在cfg中定义
func decodeAliasCmd(cmd string) (newCmd string) {
	cmdSlice := strings.Fields(cmd)
	cmdPosition0 := cmdSlice[0]
	var newCmdSlice []string
	var cmdPositionOther []string
	if len(cmdSlice) > 1 {
		cmdPositionOther = cmdSlice[1:]
	}
	for k, v := range cfg.Alias {
		if cmdPosition0 == k {
			cmdPosition0 = v
			break
		}
	}
	newCmdSlice = append(newCmdSlice, cmdPosition0)
	for _, other := range cmdPositionOther {
		newCmdSlice = append(newCmdSlice, other)
	}
	newCmd = strings.Join(newCmdSlice, " ")
	return
}

//copyAndRun 把文件拷贝到远端，并执行
//默认是拷贝到家目录下，以隐藏文件名定义
func copyAndRun(ip string, cmd string) {
	defer func() {
		wg.Done()
		<-concurrent
		if e := recover(); e != nil {
			logger.Println(e)
			return
		}
	}()
	direct := true
	client, e := connect(ip)
	if e != nil {
		logger.Println("client error:", e)
		return
	}
	defer client.Close()

	t := time.Now().Unix()
	fName := path.Base(cmd)
	destFile := "." + strconv.Itoa(int(t)) + "." + fName
	fullCmd := "./" + destFile + ";rm " + destFile

	if _, e := scp(client, cmd, destFile, direct); e != nil {
		return
	}

	session, e := client.NewSession()
	if e != nil {
		logger.Println("ssh create new session error:", e)
		return
	}
	defer session.Close()
	_, e = session.CombinedOutput("chmod 755 " + destFile)
	if e != nil {
		logger.Println(e)
		return
	}
	if cfg.Become {
		fullCmd = "sudo " + fullCmd
	}
	if session, e = client.NewSession(); e != nil {
		logger.Println("ssh create new session error:", e)
		return
	}
	out, e := session.CombinedOutput(fullCmd)
	mixOut(ip, out)

	if e != nil {
		logger.Println(ip+":", e)
		return
	}
}

//copyOnly 不指定目标时，传送到/tmp/目录下
func copyOnly(ip string, cmd string) {
	defer func() {
		wg.Done()
		<-concurrent
		if err := recover(); err != nil {
			logger.Println(err)
			return
		}
	}()
	var destFile string
	var direct bool
	cmdSlice := strings.Fields(cmd)
	srcFilePath := cmdSlice[0]
	srcFileName := path.Base(srcFilePath)
	if cfg.Become {
		direct = false
	} else {
		direct = true
	}
	if len(cmdSlice) > 1 {
		destFile = cmdSlice[1]
		if strings.HasSuffix(destFile, "/") {
			destFile = destFile + srcFileName
		}
	} else {
		destFile = "/tmp/" + srcFileName

	}
	c, e := connect(ip)
	if e != nil {
		logger.Println(e)
	}
	out, _ := scp(c, srcFilePath, destFile, direct)
	mixOut(ip, out)
}

func run(ip string, cmd string) {
	defer func() {
		wg.Done()
		<-concurrent
		if err := recover(); err != nil {
			logger.Println(err)
			return
		}
	}()
	var fullCmd string
	client, e := connect(ip)
	if e != nil {
		logger.Println(ip, ":", e)
		return
	}
	defer client.Close()

	session, e := client.NewSession()
	if e != nil {
		logger.Println("new session error:", e)
		return
	}
	defer session.Close()
	if cfg.Become {
		fullCmd = "sudo " + cmd
	} else {
		fullCmd = cmd
	}
	out, e := session.CombinedOutput(fullCmd)
	mixOut(ip, out)
	if e != nil {
		logger.Println(ip, ":", e)
		return
	}

}

func connect(str string) (client *ssh.Client, e error) {
	userPasswords := cfg.UserPasswords
	port := cfg.Sshport
	addr := str + ":" + strconv.Itoa(port)
	var content []byte
	var signer ssh.Signer
	if cfg.AuthMethod == "password" || cfg.AuthMethod == "smart" {
		for _, userPasswordsMap := range userPasswords {
			for username, password := range userPasswordsMap {
				if cfg.Debug {
					fmt.Println("ssh auth try use:", username, password)
				}
				cConfig := &ssh.ClientConfig{
					User: username,
					Auth: []ssh.AuthMethod{
						ssh.Password(password),
					},
					HostKeyCallback: ssh.InsecureIgnoreHostKey(),
					Timeout:         cfg.TimeOut,
				}
				client, e = ssh.Dial("tcp", addr, cConfig)
				if e != nil {
					if cfg.Debug {
						logger.Println(e)
					}
					continue
				} else {
					return
				}
			}
		}
	} else if cfg.AuthMethod == "sshkey" || cfg.AuthMethod == "smart" {
		for _, privateKeyMap := range cfg.PrivateKeys {
			for username, privateKey := range privateKeyMap {
				if cfg.Debug {
					fmt.Println("ssh auth try use:", username, privateKey)
				}
				content, e = ioutil.ReadFile(privateKey)
				if e != nil {
					if cfg.Debug {
						logger.Println(e)
					}
					continue
				}
				signer, e = ssh.ParsePrivateKey(content)
				cConfig := &ssh.ClientConfig{
					User: username,
					Auth: []ssh.AuthMethod{
						ssh.PublicKeys(signer),
					},
					HostKeyCallback: ssh.InsecureIgnoreHostKey(),
					Timeout:         cfg.TimeOut,
				}
				client, e = ssh.Dial("tcp", addr, cConfig)
				if e != nil {
					if cfg.Debug {
						logger.Println(e)
					}
					continue
				} else {
					return
				}
			}
		}

	} else {
		e = errors.New("ssh auth method '" + cfg.AuthMethod + "' is error")
	}
	return
}

//scp direct模式，直接scp
//非direct模式，先scp到临时目录，然后mv到目标目录
func scp(client *ssh.Client, local string, dest string, direct bool) (out []byte, e error) {
	var finfo os.FileInfo
	var backupCmd string
	var session *ssh.Session
	var destFile *sftp.File
	var tmpFile string
	addr := client.Conn.RemoteAddr().String()
	ip := strings.Split(addr, ":")[0]
	newClient, e := sftp.NewClient(client)
	if e != nil {
		logger.Print("sftp create newClient error:", e)
	}
	defer newClient.Close()

	localFile, e := os.Open(local)
	if e != nil {
		logger.Println(e)
		return
	}
	defer localFile.Close()
	// fmt.Println(dest)
	//判断目标文件是否是目录，以及如果是文件，是否要备份
	if finfo, e = newClient.Stat(dest); e != nil {

		if !os.IsNotExist(e) {
			logger.Print(e)
			return
		}
	} else {
		if finfo.IsDir() {
			srcName := path.Base(local)
			dest = dest + "/" + srcName
		}

		if cfg.BackOnCopy {
			suffix := time.Now().Format("20060102150405")
			backupFile := dest + suffix
			session, e = client.NewSession()
			if e != nil {
				logger.Print(e)
				return
			}
			if cfg.Become {
				backupCmd = "sudo cp " + dest + " " + backupFile
			} else {
				backupCmd = "cp " + dest + " " + backupFile
			}
			out, e = session.CombinedOutput(backupCmd)
			if e != nil {
				logger.Println(e)
				return
			}
		}

	}
	if direct {
		destFile, e = newClient.Create(dest)
		if e != nil {
			logger.Println(ip+" create dest file:", e)
			return
		}
		defer destFile.Close()

		_, e = io.Copy(destFile, localFile)
		if e != nil {
			logger.Println("io.Copy errr:", e)
			return
		}
	} else {
		tmpFile = time.Now().Format(".20060102150405")
		destFile, e = newClient.Create(tmpFile)
		if e != nil {
			logger.Println(ip+" create tmp file:", e)
			return
		}
		defer destFile.Close()

		_, e = io.Copy(destFile, localFile)
		if e != nil {
			logger.Println("io.Copy errr:", e)
			return
		}
		session, e = client.NewSession()
		if e != nil {
			logger.Print(e)
			return
		}
		mvCmd := "sudo mv " + tmpFile + " " + dest
		out, e = session.CombinedOutput(mvCmd)
		if e != nil {
			logger.Println(e)
			return
		}
	}
	return
}
