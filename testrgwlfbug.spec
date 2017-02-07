Name:           testrgwlfbug
Version:        1
Release:        1%{?dist}
Summary:        Test program for radosgw unsolicited linefeed bug
License:        foo

Source0:        testrgwlfbug.go
Source1:        testrgwlfbug.1

BuildRequires:       golang
BuildRequires:       golang-github-ncw-swift-devel

#ref: https://fedoraproject.org/wiki/PackagingDrafts/Go
%global _dwz_low_mem_die_limit 0

%description
This program can be used to test an instance of ceph radosgw for
precense of the stray linefeed bug.  Older copies of radosgw
emitted a newline after the body of certain requests that was not
accounted for in the content length.  The go SDK for swift,
ncw/swift, finds this extra newline and emits a scary message
to stderr.  We, the ceph developers, do not want our code
to cause such messages and the current version of ceph has
been corrected to stop that.  This program can test to verify
that the fix has been properly implemented and is functioning
as designed.

%prep
%setup -c -T
cp -p %SOURCE0 .

%build
#GOPATH=/usr/share/gocode LDFLAGS="-X %{import_path}/version.GitSHA=%{shortcommit}" go build testrgwlfbug.go
GOPATH=/usr/share/gocode go build -ldflags "-B 0x`head -c20 /dev/urandom|od -An -tx1|tr -d ' \n'`" testrgwlfbug.go

%install
install -dm 755 %{buildroot}%{_bindir}
install -dm 755 %{buildroot}%{_mandir}/man1
install -pm 755 testrgwlfbug %{buildroot}%{_bindir}/testrgwlfbug
install -pm 755 %{SOURCE1} %{buildroot}%{_mandir}/man1/testrgwlfbug.1

%files
%defattr(-,root,root,-)
%{_bindir}/testrgwlfbug
%{_mandir}/man1/testrgwlfbug.1*

%changelog
* Tue Feb  7 2017 Marcus Watts <mwatts@redhat.com> - 1-1
- package it
