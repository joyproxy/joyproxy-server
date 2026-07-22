#!/usr/bin/env perl
# mini-PWA: auth for joyproxy SPS. Logs denied (NO) sessions only.
#
# Mode A (single egress): --whitelist --users --denykey --log
# Mode B (multi egress):  --auth-dir /root/auth --denykey /root/auth/denykey.txt --log
#   /root/auth/117.89.88.141/iplist.txt  users.txt  (per public IP)
#   /root/auth/denykey.txt  shared by all egresses
#   /root/auth/default/  fallback when local_addr unknown
use strict;
use warnings;
use IO::Socket::INET;
use Getopt::Long qw(GetOptions);
use Fcntl qw(:flock SEEK_END);
use POSIX qw(WNOHANG);

my $LISTEN     = '127.0.0.1:6301';
my $AUTH_DIR   = '';
my $WHITELIST  = '';
my $USERS      = '';
my $DENYKEY    = '';
my $LOGFILE    = '';
my $EIPMAP     = '';
my $RELOAD_SEC = 30;

my (%whitelist, %users, @denykeys);
my (%egress_priv, $egress_mtime);
my ($wl_mtime, $usr_mtime, $dk_mtime) = (0, 0, 0);
my %profile_cache;
my (@shared_denykeys, $shared_dk_mtime);

sub usage {
    print STDERR <<'USAGE';
Usage: minipwa.pl [options]

Mode A (single egress):
  --whitelist FILE  --users FILE  --denykey FILE  --log FILE

Mode B (multi egress, per public IP):
  --auth-dir DIR    e.g. /root/auth
  --denykey FILE    shared deny list, e.g. /root/auth/denykey.txt
  --log FILE
  per egress: DIR/<public-ip>/iplist.txt  DIR/<public-ip>/users.txt
  fallback:   DIR/default/iplist.txt users.txt

Optional: --listen HOST:PORT  --eipmap FILE  --reload SEC
  Multi-egress: returns outgoing: <内网IP> from eipmap for joyproxy dial bind

Auth: valid user+pass -> else whitelist IP -> then denykey on target
  Same username may appear on multiple lines with different passwords (all valid).
USAGE
    exit 1;
}

GetOptions(
    'listen=s'    => \$LISTEN,
    'auth-dir=s'  => \$AUTH_DIR,
    'whitelist=s' => \$WHITELIST,
    'users=s'     => \$USERS,
    'denykey=s'   => \$DENYKEY,
    'log=s'       => \$LOGFILE,
    'eipmap=s'    => \$EIPMAP,
    'reload=i'    => \$RELOAD_SEC,
) or usage();

$EIPMAP = '/root/eipmap.txt' if $AUTH_DIR ne '' && $EIPMAP eq '';

usage() if !defined $LOGFILE || $LOGFILE eq '';

if ($AUTH_DIR ne '') {
    usage() if !-d $AUTH_DIR;
    usage() if !defined $DENYKEY || $DENYKEY eq '';
}
else {
    for my $pair (
        ['--whitelist', $WHITELIST],
        ['--users',     $USERS],
        ['--denykey',   $DENYKEY],
    ) {
        my ($name, $val) = @$pair;
        usage() if !defined $val || $val eq '';
    }
}

sub now_ms { int(time() * 1000) }

$SIG{CHLD} = sub {
    while ((my $pid = waitpid(-1, WNOHANG)) > 0) { }
};
$SIG{HUP} = sub {
    %profile_cache = ();
    %egress_priv = ();
    $egress_mtime = 0;
    $shared_dk_mtime = 0;
    if ($AUTH_DIR eq '') {
        reload_single_files(1);
    }
    else {
        load_shared_denykey(1);
        warn "minipwa: auth-dir cache cleared\n";
    }
};

sub load_lines {
    my ($path) = @_;
    my @out;
    return \@out if !-r $path;
    open my $fh, '<', $path or return \@out;
    while (my $line = <$fh>) {
        $line =~ s/\r?\n\z//;
        $line =~ s/#.*\z//;
        $line =~ s/^\s+|\s+$//g;
        next if $line eq '';
        push @out, $line;
    }
    close $fh;
    return \@out;
}

sub parse_whitelist_ip {
    my ($line) = @_;
    if ($line =~ /^([\d.]+)(?::.*)?$/) {
        return $1;
    }
    if ($line =~ /^\[([0-9a-fA-F:]+)\](?::.*)?$/) {
        return $1;
    }
    return $line;
}

sub load_shared_denykey {
    my ($force_log) = @_;
    my $dk_m = (stat($DENYKEY))[9] // 0;
    return if !$force_log && $dk_m == $shared_dk_mtime;
    @shared_denykeys = @{ load_lines($DENYKEY) };
    $shared_dk_mtime = $dk_m;
    warn sprintf("minipwa reload shared denykey=%d\n", scalar(@shared_denykeys)) if $force_log;
}

sub load_profile_wl_usr {
    my ($wl_path, $usr_path) = @_;
    my %wl;
    for my $line (@{ load_lines($wl_path) }) {
        my $ip = parse_whitelist_ip($line);
        $wl{$ip} = 1 if $ip ne '';
    }
    my %usr;
    for my $line (@{ load_lines($usr_path) }) {
        my ($u, $p) = split /:/, $line, 2;
        next if !defined $u || $u eq '' || !defined $p;
        $usr{$u}{$p} = 1;
    }
    return {
        whitelist => \%wl,
        users     => \%usr,
        mtime     => {
            wl  => (stat($wl_path))[9]  // 0,
            usr => (stat($usr_path))[9] // 0,
        },
    };
}

sub load_into_profile {
    my ($wl_path, $usr_path, $dk_path) = @_;
    my $p = load_profile_wl_usr($wl_path, $usr_path);
    my @dk = @{ load_lines($dk_path) };
    $p->{denykeys} = \@dk;
    $p->{mtime}{dk} = (stat($dk_path))[9] // 0;
    return $p;
}

sub reload_single_files {
    my ($force_log) = @_;
    my $wl_m  = (stat($WHITELIST))[9] // 0;
    my $usr_m = (stat($USERS))[9]     // 0;
    my $dk_m  = (stat($DENYKEY))[9]   // 0;
    return if !$force_log && $wl_m == $wl_mtime && $usr_m == $usr_mtime && $dk_m == $dk_mtime;

    my $p = load_into_profile($WHITELIST, $USERS, $DENYKEY);
    %whitelist = %{ $p->{whitelist} };
    %users     = %{ $p->{users} };
    @denykeys  = @{ $p->{denykeys} };
    $wl_mtime  = $wl_m;
    $usr_mtime = $usr_m;
    $dk_mtime  = $dk_m;

    if ($force_log) {
        warn sprintf(
            "minipwa reload whitelist=%d users=%d denykey=%d\n",
            scalar(keys %whitelist),
            scalar(keys %users),
            scalar(@denykeys),
        );
    }
}

sub addr_host {
    my ($addr) = @_;
    return '' if !defined $addr || $addr eq '';
    if ($addr =~ /^\[(.+)\]:(\d+)$/) {
        return $1;
    }
    my ($ip) = split /:/, $addr, 2;
    return $ip // '';
}

sub profile_key {
    my ($local_addr) = @_;
    return 'single' if $AUTH_DIR eq '';

    my $ip = addr_host($local_addr);
    $ip = '' if $ip eq '0.0.0.0';

    if ($ip ne '' && profile_dir_exists($ip)) {
        return $ip;
    }
    if (profile_dir_exists('default')) {
        return 'default';
    }
    return $ip ne '' ? $ip : 'default';
}

sub profile_dir_exists {
    my ($key) = @_;
    my $base = "$AUTH_DIR/$key";
    return 1 if -d $base;
    return 1 if -e "$base/iplist.txt" || -e "$base/users.txt";
    return 0;
}

sub profile_paths {
    my ($key) = @_;
    if ($AUTH_DIR ne '') {
        my $base = "$AUTH_DIR/$key";
        return (
            wl => "$base/iplist.txt",
            usr => "$base/users.txt",
        );
    }
    return (wl => $WHITELIST, usr => $USERS, dk => $DENYKEY);
}

sub get_profile {
    my ($local_addr) = @_;
    if ($AUTH_DIR eq '') {
        reload_single_files(0);
        return {
            key       => 'single',
            whitelist => \%whitelist,
            users     => \%users,
            denykeys  => \@denykeys,
        };
    }

    load_shared_denykey(0);
    my $key = profile_key($local_addr);
    my %paths = profile_paths($key);
    my $c = $profile_cache{$key};
    my $wl_m  = (stat($paths{wl}))[9]  // 0;
    my $usr_m = (stat($paths{usr}))[9] // 0;

    if ($c && $c->{mtime}{wl} == $wl_m && $c->{mtime}{usr} == $usr_m
        && $c->{shared_dk_mtime} // 0 == $shared_dk_mtime)
    {
        return $c;
    }

    $c = load_profile_wl_usr($paths{wl}, $paths{usr});
    $c->{key} = $key;
    $c->{denykeys} = \@shared_denykeys;
    $c->{shared_dk_mtime} = $shared_dk_mtime;
    $profile_cache{$key} = $c;
    return $c;
}

sub client_ip {
    return addr_host($_[0]);
}

sub load_egress_map {
    return if $EIPMAP eq '' || !-r $EIPMAP;
    my $m = (stat($EIPMAP))[9] // 0;
    return if $m == $egress_mtime;
    %egress_priv = ();
    for my $line (@{ load_lines($EIPMAP) }) {
        my ($pub, $priv) = split /\s+/, $line, 2;
        next if !defined $pub || $pub eq '' || $pub =~ /^#/;
        next if !defined $priv || $priv eq '' || $priv eq '0.0.0.0';
        $egress_priv{$pub} = $priv;
    }
    $egress_mtime = $m;
}

sub egress_outgoing {
    my ($local_addr) = @_;
    load_egress_map();
    my $pub = addr_host($local_addr);
    return '' if $pub eq '' || $pub eq '0.0.0.0';
    return $egress_priv{$pub} // '';
}

sub format_target {
    my ($target) = @_;
    return '0.0.0.0:0' if !defined $target || $target eq '';

    if ($target =~ m{^https?://([^/?#:]+)(?::(\d+))?}i) {
        my ($host, $port) = ($1, $2);
        if (!defined $port || $port eq '') {
            $port = ($target =~ m{^https://}i) ? 443 : 80;
        }
        return "$host:$port";
    }
    if ($target =~ /^([^:]+):(\d+)$/) {
        return "$1:$2";
    }
    return $target;
}

sub target_host {
    my ($target) = @_;
    return '' if !defined $target || $target eq '';
    if ($target =~ m{^https?://([^/?#:]+)}i) {
        return lc $1;
    }
    if ($target =~ /^([^:]+):\d+$/) {
        return lc $1;
    }
    return lc $target;
}

sub target_denied {
    my ($target, $denykeys) = @_;
    return 0 if !defined $target || $target eq '';
    my $host = target_host($target);
    my $full = lc $target;

    for my $key (@$denykeys) {
        my $k = lc $key;
        next if $k eq '';

        if ($k =~ /^\d+\.\d+\.\d+\.\d+$/) {
            return 1 if $host eq $k;
            return 1 if index($full, $k) >= 0;
            next;
        }
        if (substr($k, 0, 1) eq '.') {
            return 1 if $host =~ /\Q$k\E\z/i;
            next;
        }
        return 1 if $host eq $k;
        return 1 if $host =~ /\.\Q$k\E\z/i;
        return 1 if index($host, $k) >= 0;
        return 1 if index($full, $k) >= 0;
    }
    return 0;
}

sub proxy_proto {
    my ($service) = @_;
    return ($service && lc($service) eq 'http') ? 'HTTP' : 'S5';
}

sub comm_proto {
    my ($service, $target) = @_;
    if ($service && lc($service) eq 'socks' && (!defined $target || $target eq '')) {
        return 'UDP';
    }
    return 'TCP';
}

sub append_log {
    my ($line) = @_;
    open my $fh, '>>', $LOGFILE or do {
        warn "minipwa cannot open log $LOGFILE: $!\n";
        return;
    };
    flock($fh, LOCK_EX);
    seek($fh, 0, SEEK_END);
    print {$fh} $line, "\n";
    flock($fh, LOCK_UN);
    close $fh;
}

sub write_no_log {
    my (%p) = @_;
    my $ts = now_ms();
    my $user = (defined $p{user} && $p{user} ne '') ? $p{user} : '-';
    my $line = join ' ',
        $ts,
        'NO',
        proxy_proto($p{service}),
        comm_proto($p{service}, $p{target}),
        $p{client_addr},
        $p{local_addr},
        '-:-',
        format_target($p{target}),
        $user,
        0;
    append_log($line);
}

sub parse_query {
    my ($query) = @_;
    my %q;
    for my $pair (split /&/, $query // '') {
        next if $pair eq '';
        my ($k, $v) = split /=/, $pair, 2;
        $k =~ tr/+/ /;
        $v =~ tr/+/ / if defined $v;
        $k =~ s/%([0-9A-Fa-f]{2})/chr(hex($1))/ge;
        $v =~ s/%([0-9A-Fa-f]{2})/chr(hex($1))/ge if defined $v;
        $v = '' if !defined $v;
        $q{$k} = $v;
    }
    return %q;
}

sub check_identity {
    my ($user, $pass, $client_addr, $prof) = @_;
    my $has_creds = defined $user && $user ne '' && defined $pass && $pass ne '';
    my %wl   = %{ $prof->{whitelist} };
    my %usr  = %{ $prof->{users} };

    if ($has_creds && exists $usr{$user} && $usr{$user}{$pass}) {
        return (1, '');
    }

    my $cli_ip = client_ip($client_addr);
    if ($cli_ip ne '' && $wl{$cli_ip}) {
        return (1, '');
    }

    return (0, $has_creds ? 'auth' : 'whitelist');
}

sub handle_auth {
    my (%q) = @_;
    my $user        = $q{user}        // '';
    my $pass        = $q{pass}        // '';
    my $client_addr = $q{client_addr} // '';
    my $local_addr  = $q{local_addr}  // '';
    my $target      = $q{target}      // '';
    my $service     = $q{service}     // '';

    my $prof = get_profile($local_addr);

    my ($ok, $deny) = ('OK', '');
    my ($id_ok, $id_deny) = check_identity($user, $pass, $client_addr, $prof);
    if (!$id_ok) {
        $ok   = 'NO';
        $deny = $id_deny;
    }
    elsif (target_denied($target, $prof->{denykeys})) {
        $ok   = 'NO';
        $deny = 'forbidden';
    }

    if ($ok eq 'NO') {
        write_no_log(
            user        => $user,
            client_addr => $client_addr,
            local_addr  => $local_addr,
            target      => $target,
            service     => $service,
        );
        my $deny_hdr = $deny eq 'forbidden' ? 'forbidden' : $deny;
        return "HTTP/1.1 204 No Content\r\n"
             . "upstream: ERR\r\n"
             . "X-Joyproxy-Deny: $deny_hdr\r\n"
             . "Connection: close\r\n\r\n";
    }

    my $out = egress_outgoing($local_addr);
    my $hdr = "HTTP/1.1 204 No Content\r\n";
    $hdr .= "outgoing: $out\r\n" if $out ne '';
    $hdr .= "Connection: close\r\n\r\n";
    return $hdr;
}

sub handle_client {
    my ($client) = @_;
    local $SIG{CHLD} = 'IGNORE';
    my $pid = fork();
    if (!defined $pid) {
        close $client;
        return;
    }
    if ($pid != 0) {
        close $client;
        return;
    }

    my $req = '';
    while ($client->sysread(my $buf, 4096)) {
        $req .= $buf;
        last if length($req) > 65536;
        last if $req =~ /\r\n\r\n/;
    }

    my ($req_line) = split /\r\n/, $req, 2;
    $req_line //= '';
    my ($method, $path) = split / /, $req_line, 3;

    my $resp;
    if (!defined $method || uc($method) ne 'GET') {
        $resp = "HTTP/1.1 405 Method Not Allowed\r\nConnection: close\r\n\r\n";
    }
    else {
        my ($p, $query) = split /\?/, $path, 2;
        $p //= '/';
        if ($p eq '/health' || $p eq '/ping') {
            $resp = "HTTP/1.1 204 No Content\r\nConnection: close\r\n\r\n";
        }
        else {
            my %q = parse_query($query);
            $resp = handle_auth(%q);
        }
    }

    print {$client} $resp;
    close $client;
    exit 0;
}

sub main {
    if ($AUTH_DIR eq '') {
        for my $pair (
            ['whitelist', $WHITELIST],
            ['users',     $USERS],
            ['denykey',   $DENYKEY],
        ) {
            my ($label, $path) = @$pair;
            die "minipwa: $label not found: $path\n" if !-e $path;
            die "minipwa: $label not readable: $path\n" if !-r $path;
        }
        reload_single_files(1);
    }
    else {
        die "minipwa: denykey not found: $DENYKEY\n" if !-e $DENYKEY;
        die "minipwa: denykey not readable: $DENYKEY\n" if !-r $DENYKEY;
        load_shared_denykey(1);
        if (!profile_dir_exists('default')) {
            warn "minipwa: warn: $AUTH_DIR/default missing, use per-public-ip dirs\n";
        }
    }

    if (!-e $LOGFILE) {
        open my $t, '>>', $LOGFILE or die "minipwa: cannot create log $LOGFILE: $!\n";
        close $t;
    }

    my ($host, $port) = split /:/, $LISTEN, 2;
    $host = '127.0.0.1' if !defined $host || $host eq '';
    $port = 6301         if !defined $port || $port eq '';

    my $server = IO::Socket::INET->new(
        LocalHost => $host,
        LocalPort => $port,
        Proto     => 'tcp',
        Listen    => 128,
        Reuse     => 1,
    ) or die "minipwa listen $LISTEN: $!\n";

    if ($AUTH_DIR ne '') {
        warn "minipwa listen=$LISTEN auth-dir=$AUTH_DIR denykey=$DENYKEY log=$LOGFILE eipmap=$EIPMAP\n";
    }
    else {
        warn "minipwa listen=$LISTEN whitelist=$WHITELIST users=$USERS denykey=$DENYKEY log=$LOGFILE\n";
    }

    while (1) {
        my $c = $server->accept();
        next if !defined $c;
        handle_client($c);
    }
}

main();
