 package whois

 import (
	 "fmt"
	 "io"
	 "net"
	 "strconv"
	 "strings"
	 "time"
     "log"
 
	 "golang.org/x/net/proxy"
	//  "github.com/go-httpproxy/httpproxy"
	 "net/url"
    //  utls "github.com/refraction-networking/utls"
     "github.com/magisterquis/connectproxy"
 )
 
 const (
	 // defaultWhoisServer is iana whois server
	 defaultWhoisServer = "whois.iana.org"
	 // defaultWhoisPort is default whois port
	 defaultWhoisPort = "43"
	 // defaultTimeout is query default timeout
	 defaultTimeout = 30 * time.Second
	 // asnPrefix is asn prefix string
	 asnPrefix = "AS"
 )
 
 // DefaultClient is default whois client
 var DefaultClient = NewClient()
 
 // Client is whois client
 type Client struct {
	 dialer          proxy.Dialer
	 timeout         time.Duration
	 elapsed         time.Duration
	 disableStats    bool
	 disableReferral bool
 }

 
 // Version returns package version
 func Version() string {
	 return "1.15.3"
 }
 
 // Author returns package author
 func Author() string {
	 return "[Li Kexian](https://www.likexian.com/)"
 }
 
 // License returns package license
 func License() string {
	 return "Licensed under the Apache License 2.0"
 }
 
 // Whois do the whois query and returns whois information
 func Whois(domain string, proxyURL string, servers ...string) (result string, err error) {
	 return DefaultClient.Whois(domain, proxyURL, servers...)
 }
 
 // NewClient returns new whois client
 func NewClient() *Client {
	 return &Client{
		 dialer: &net.Dialer{
			 Timeout: defaultTimeout,
		 },
		 timeout: defaultTimeout,
	 }
 }
 
 // SetDialer set query net dialer
 func (c *Client) SetDialer(dialer proxy.Dialer) *Client {
	 c.dialer = dialer
	 return c
 }
 
 // SetTimeout set query timeout
 func (c *Client) SetTimeout(timeout time.Duration) *Client {
	 c.timeout = timeout
	 return c
 }
 
 // SetDisableStats set disable stats
 func (c *Client) SetDisableStats(disabled bool) *Client {
	 c.disableStats = disabled
	 return c
 }
 
 // SetDisableReferral if set to true, will not query the referral server.
 func (c *Client) SetDisableReferral(disabled bool) *Client {
	 c.disableReferral = disabled
	 return c
 }


func (c *Client) Whois(domain string, proxyURL string, servers ...string) (result string, err error) {
    start := time.Now()
    defer func() {
        result = strings.TrimSpace(result)
        if result != "" && !c.disableStats {
            result = fmt.Sprintf("%s\n\n%% Query time: %d msec\n%% WHEN: %s\n",
                result, time.Since(start).Milliseconds(), start.Format("Mon Jan 02 15:04:05 MST 2006"),
            )
        }
    }()

    domain = strings.Trim(strings.TrimSpace(domain), ".")
    if domain == "" {
        return "", ErrDomainEmpty
    }

    isASN := IsASN(domain)
    if isASN {
        if !strings.HasPrefix(strings.ToUpper(domain), asnPrefix) {
            domain = asnPrefix + domain
        }
    }

    if !strings.Contains(domain, ".") && !strings.Contains(domain, ":") && !isASN {
        return c.rawQuery(domain, defaultWhoisServer, defaultWhoisPort, proxyURL)
    }

    var server, port string
    if len(servers) > 0 && servers[0] != "" {
        server = strings.ToLower(servers[0])
        port = defaultWhoisPort
    } else {
        ext := getExtension(domain)
        result, err := c.rawQuery(ext, defaultWhoisServer, defaultWhoisPort, proxyURL)
        if err != nil {
            return "", fmt.Errorf("whois: query for whois server failed: %w", err)
        }
        server, port = getServer(result)
        if server == "" {
            return "", fmt.Errorf("%w: %s", ErrWhoisServerNotFound, domain)
        }
    }

    result, err = c.rawQuery(domain, server, port, proxyURL)
    if err != nil {
        return
    }

    if c.disableReferral {
        return
    }

    refServer, refPort := getServer(result)
    if refServer == "" || refServer == server {
        return
    }

    data, err := c.rawQuery(domain, refServer, refPort, proxyURL)
    if err == nil {
        result += data
    }

    return
}


func (c *Client) rawQuery(domain, server, port, proxyURL string) (string, error) {
    c.elapsed = 0
    start := time.Now()

    if server == "whois.arin.net" {
        if IsASN(domain) {
            domain = "a + " + domain
        } else {
            domain = "n + " + domain
        }
    }

    // See: https://github.com/likexian/whois/issues/17
    if server == "whois.godaddy" {
        server = "whois.godaddy.com"
    }

    // See: https://github.com/likexian/whois/pull/30
    if server == "porkbun.com/whois" {
        server = "whois.porkbun.com"
    }
   
    var conn net.Conn
    var err error
    var proxyDialer proxy.Dialer

    dialer := &net.Dialer{
        Timeout: c.timeout,
    }

    if proxyURL != "" {

        proxyURI, _ := url.Parse(proxyURL)
    
        switch proxyURI.Scheme {                                                       
        case "socks5":                                                                 
                // proxyDialer, err = proxy.SOCKS5("tcp", proxyURL, nil, proxy.Direct)
                proxyDialer, err = proxy.SOCKS5("tcp", proxyURI.Host, nil, dialer)
        case "http":                                                          
                // proxyDialer, err = connectproxy.New(proxyURI, proxy.Direct)
                proxyDialer, err = connectproxy.New(proxyURI, dialer)      
        }    
        conn, err = proxyDialer.Dial("tcp", net.JoinHostPort(server, port))
        if err != nil {
            return "", fmt.Errorf("whois: connect to whois server failed: %w", err)
        }

        defer func() {
            if closeErr := conn.Close(); closeErr != nil {
                log.Printf("Error closing connection: %v", closeErr)
            }
        }()
    
        c.elapsed = time.Since(start)
    
        _ = conn.SetWriteDeadline(time.Now().Add(c.timeout - c.elapsed))
        _, err = conn.Write([]byte(domain + "\r\n"))
        if err != nil {
            return "", fmt.Errorf("whois: send to whois server failed: %w", err)
        }
    
        c.elapsed = time.Since(start)
    
        _ = conn.SetReadDeadline(time.Now().Add(c.timeout - c.elapsed))
        buffer, err := io.ReadAll(conn)
        if err != nil {
            return "", fmt.Errorf("whois: read from whois server failed: %w", err)
        }
    
        c.elapsed = time.Since(start)
    
        return string(buffer), nil

    

    } else {
        conn, err = dialer.Dial("tcp", net.JoinHostPort(server, port))
        if err != nil {
            return "", fmt.Errorf("whois: connect to whois server failed: %w", err)
        }

        defer func() {
            if closeErr := conn.Close(); closeErr != nil {
                log.Printf("Error closing connection: %v", closeErr)
            }
        }()
    
        c.elapsed = time.Since(start)
    
        _ = conn.SetWriteDeadline(time.Now().Add(c.timeout - c.elapsed))
        _, err = conn.Write([]byte(domain + "\r\n"))
        if err != nil {
            return "", fmt.Errorf("whois: send to whois server failed: %w", err)
        }
    
        c.elapsed = time.Since(start)
    
        _ = conn.SetReadDeadline(time.Now().Add(c.timeout - c.elapsed))
        buffer, err := io.ReadAll(conn)
        if err != nil {
            return "", fmt.Errorf("whois: read from whois server failed: %w", err)
        }
    
        c.elapsed = time.Since(start)
    
        return string(buffer), nil
    }

    return "", nil
}

//result, err := c.Whois("example.com", "http://user:password@proxy.example.com:8080")


 
 // getExtension returns extension of domain
 func getExtension(domain string) string {
	 ext := domain
 
	 if net.ParseIP(domain) == nil {
		 domains := strings.Split(domain, ".")
		 ext = domains[len(domains)-1]
	 }
 
	 if strings.Contains(ext, "/") {
		 ext = strings.Split(ext, "/")[0]
	 }
 
	 return ext
 }
 
 // getServer returns server from whois data
 func getServer(data string) (string, string) {
	 tokens := []string{
		 "Registrar WHOIS Server: ",
		 "whois: ",
		 "ReferralServer: ",
		 "refer: ",
	 }
 
	 for _, token := range tokens {
		 start := strings.Index(data, token)
		 if start != -1 {
			 start += len(token)
			 end := strings.Index(data[start:], "\n")
			 server := strings.TrimSpace(data[start : start+end])
			 server = strings.TrimPrefix(server, "http:")
			 server = strings.TrimPrefix(server, "https:")
			 server = strings.TrimPrefix(server, "whois:")
			 server = strings.TrimPrefix(server, "rwhois:")
			 server = strings.Trim(server, "/")
			 port := defaultWhoisPort
			 if strings.Contains(server, ":") {
				 v := strings.Split(server, ":")
				 server, port = v[0], v[1]
			 }
			 return server, port
		 }
	 }
 
	 return "", ""
 }
 
 // IsASN returns if s is ASN
 func IsASN(s string) bool {
	 s = strings.ToUpper(s)
 
	 s = strings.TrimPrefix(s, asnPrefix)
	 _, err := strconv.Atoi(s)
 
	 return err == nil
 }
