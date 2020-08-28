#!/bin/bash

NAME=relaymon
URL="https://github.com/msaf1980/relaymon"
DESCR="${NAME}: graphite relay monitor"
LICENSE="MIT"

if [ -z "$1" ]; then
	echo "use: $0 [rpm | deb]"
	exit 1
fi

die() {
    if [[ $1 -eq 0 ]]; then
        [ -d "${TMPDIR}" ] && rm -rf "${TMPDIR}"
    else
        [ "${TMPDIR}" = "" ] || echo "Temporary data stored at '${TMPDIR}'"
    fi
    echo "$2"
    exit $1
}

pwd

GIT_VERSION="$(git describe --always --tags)" || die 1 "Can't get latest version from git"

set -f; IFS='-' ; arr=($GIT_VERSION)
VERSION=${arr[0]}; [ -z "${arr[2]}" ] && RELEASE=${arr[1]} || RELEASE=${arr[1]}.${arr[2]}
set +f; unset IFS

[ "$RELEASE" == "" -a "$VERSION" != "" ] && RELEASE=0

if ! echo $VERSION | egrep '^v[0-9]+\.[0-9]+(\.[0-9]+)?$' >/dev/null; then
	echo "Revision: ${RELEASE}";
	echo "Version: ${VERSION}";
	echo
	echo "Known tags:"
	git tag
	echo;
	die 1 "Can't parse version from git";
fi

[ -r "relaymon" ] || {
    make || die 1 "Build error"
}

package() {
	TMPDIR=$(mktemp -d)
	[ "${TMPDIR}" = "" ] && die 1 "Can't create temp dir"
	echo version ${VERSION} release ${RELEASE}

	mkdir -p "${TMPDIR}/usr/bin" || die 1 "Can't create bin dir"
	mkdir -p "${TMPDIR}/usr/share/${NAME}" || die 1 "Can't create share dir"
	mkdir -p "${TMPDIR}/usr/lib/systemd/system" || die 1 "Can't create systemd dir"
	cp ./${NAME} "${TMPDIR}/usr/bin/" || die 1 "Can't install package binary"
	cp ./${NAME}.yml "${TMPDIR}/usr/share/${NAME}/" || die 1 "Can't install package shared files"

	if [ "$1" == "rpm" ]; then
		mkdir -p "${TMPDIR}/etc/sysconfig" || die 1 "Can't create sysconfig dir"
		cp ./contrib/rpm/${NAME}.service "${TMPDIR}/usr/lib/systemd/system" || die 1 "Can't install package systemd files"
		cp ./contrib/common/${NAME}.env "${TMPDIR}/etc/sysconfig/${NAME}" || die 1 "Can't install package sysconfig file"
		fpm -s dir -t rpm -n ${NAME} -v ${VERSION} -C ${TMPDIR} \
			--iteration ${RELEASE} \
			-p ${NAME}-VERSION-ITERATION.ARCH.rpm \
			--description "${DESCR}" \
			--license "${LICENSE}" \
			--url "${URL}" \
			etc usr/bin usr/lib/systemd usr/share || die 1 "Can't create package!"
	else
		mkdir -p "${TMPDIR}/etc/default" || die 1 "Can't create sysconfig dir"
		cp ./contrib/common/${NAME}.env "${TMPDIR}/etc/default/${NAME}" || die 1 "Can't install package sysconfig file"
		cp ./contrib/deb/${NAME}.service "${TMPDIR}/usr/lib/systemd/system" || die 1 "Can't install package systemd files"
		fpm -s dir -t deb -n ${NAME} -v ${VERSION} -C ${TMPDIR} \
			--iteration ${RELEASE} \
			-p ${NAME}-VERSION-ITERATION.ARCH.deb \
			--description "${DESR}" \
			--license "${LICENSE}" \
			--url "${URL}" \
			etc usr/bin usr/lib/systemd usr/share || die 1 "Can't create deb package!"
	fi

    rm -rf "${TMPDIR}"
}

while [ $1 ]; do
    case "$1" in
		"rpm")
			package "$1"
			;;
		"deb")
			package "deb"
			;;
		*)
			echo "unknown format"
			exit 1
	esac
	shift
done

die 0 "Success"
