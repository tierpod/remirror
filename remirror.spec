
Name:        remirror
Version:     %{_version}
Release:     %{_release}
Summary:     An aggressively caching proxy for artifact mirroring
Group:       System Environment/Daemons
License:     MIT
BuildArch:   x86_64

# no source rpms
%define __os_install_post %{nil}

# dont do magic jar stuff
%define __osgi_provides %{nil}
%define __osgi_requires %{nil}

%description

%prep

%build

%install

mkdir -p %{buildroot}/usr/bin
mkdir -p %{buildroot}/lib/systemd/system

cp %{_origin}/remirror         %{buildroot}/usr/bin/
cp %{_origin}/remirror.service %{buildroot}/lib/systemd/system/

%files
%attr(0755, root, root) /usr/bin/remirror
%attr(0644, root, root) /lib/systemd/system/remirror.service

%post
/usr/sbin/setcap cap_net_bind_service=+ep /usr/bin/remirror

