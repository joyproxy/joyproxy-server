#!/usr/bin/env perl
# mini-PWL: traffic log for joyproxy SPS. Logs successful (OK) sessions only.
use strict;
use warnings;
use IO::Socket::INET;
use Getopt::Long qw(GetOptions);
use Fcntl qw(:flock SEEK_END);
use POSIX qw(WNOHANG);

my $LISTEN  = '127.0.0.1:6303';
my $LOGFILE = '';

sub usage {
    print STDERR <<'USAGE';
Usage: minipwl.pl [options]

Required:
  --log FILE             log file for OK lines (e.g. /root/s9cron.log)

Optional:
  --listen HOST:PORT     default 127.0.0.1:6303

SPS v2.2:
  --traffic-url "http://127.0.0.1:6303/traffic"
USAGE
    exit 1;
}

GetOptions(
    'listen=s' => \$LISTEN,
    'log=s'    => \$LOGFILE,
) or usage();

usage() if !defined $LOGFILE || $LOGFILE eq '';

sub now_ms { int(time() * 1000) }

$SIG{CHLD} = sub {
    while ((my $pid = waitpid(-1, WNOHANG)) > 0) { }
};

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

sub proxy_proto {
    my ($id) = @_;
    return ($id && lc($id) eq 'http') ? 'HTTP' : 'S5';
}

sub comm_proto {
    my ($id, $target) = @_;
    if ($id && lc($id) eq 'socks' && (!defined $target || $target eq '')) {
        return 'UDP';
    }
    return 'TCP';
}

sub norm_addr {
    my ($addr) = @_;
    return '-:-' if !defined $addr || $addr eq '';
    return $addr;
}

sub append_log {
    my ($line) = @_;
    open my $fh, '>>', $LOGFILE or do {
        warn "minipwl cannot open log $LOGFILE: $!\n";
        return;
    };
    flock($fh, LOCK_EX);
    seek($fh, 0, SEEK_END);
    print {$fh} $line, "\n";
    flock($fh, LOCK_UN);
    close $fh;
}

sub write_ok_log {
    my (%p) = @_;
    my $ts = now_ms();
    my $user = (defined $p{username} && $p{username} ne '') ? $p{username} : '-';
    my $bytes = (defined $p{bytes} && $p{bytes} ne '') ? $p{bytes} : '0';
    my $line = join ' ',
        $ts,
        'OK',
        proxy_proto($p{id}),
        comm_proto($p{id}, $p{target_addr}),
        norm_addr($p{client_addr}),
        norm_addr($p{server_addr}),
        norm_addr($p{out_local_addr}),
        format_target($p{target_addr}),
        $user,
        $bytes;
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

sub handle_traffic {
    my (%q) = @_;
    write_ok_log(
        bytes          => $q{bytes}          // '0',
        client_addr    => $q{client_addr}    // '',
        server_addr    => $q{server_addr}    // '',
        target_addr    => $q{target_addr}    // '',
        username       => $q{username}       // '',
        out_local_addr => $q{out_local_addr} // '',
        id             => $q{id}             // '',
    );
    return "HTTP/1.1 204 No Content\r\nConnection: close\r\n\r\n";
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
        elsif ($p eq '/traffic' || $p eq '/traffic/') {
            my %q = parse_query($query);
            $resp = handle_traffic(%q);
        }
        else {
            $resp = "HTTP/1.1 404 Not Found\r\nConnection: close\r\n\r\n";
        }
    }

    print {$client} $resp;
    close $client;
    exit 0;
}

sub main {
    if (!-e $LOGFILE) {
        open my $t, '>>', $LOGFILE or die "minipwl: cannot create log $LOGFILE: $!\n";
        close $t;
    }

    my ($host, $port) = split /:/, $LISTEN, 2;
    $host = '127.0.0.1' if !defined $host || $host eq '';
    $port = 6303         if !defined $port || $port eq '';

    my $server = IO::Socket::INET->new(
        LocalHost => $host,
        LocalPort => $port,
        Proto     => 'tcp',
        Listen    => 128,
        Reuse     => 1,
    ) or die "minipwl listen $LISTEN: $!\n";

    warn "minipwl listen=$LISTEN log=$LOGFILE\n";

    while (1) {
        my $c = $server->accept();
        next if !defined $c;
        handle_client($c);
    }
}

main();
