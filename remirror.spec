Name:        remirror
Version:     %{_version}
Release:     %{_release}
Summary:     Experticity local proxy/mirror
Group:       Vendor/Experticity
License:     Proprietary
BuildArch:   noarch

# no source rpms
%define __os_install_post %{nil}

%description
%{_description}

%prep

%build

%install

mkdir -p %{buildroot}/usr/bin
mkdir -p %{buildroot}/lib/systemd/system

cp remirror          %{buildroot}/usr/bin
cp remirror.service  %{buildroot}/lib/systemd/system

%files

%attr(0755,root,root)  /usr/bin/remirror
                       /lib/systemd/system/remirror.service

