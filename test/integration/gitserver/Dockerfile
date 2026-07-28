# Minimal git-over-SSH server used only by the integration test suite
# (test/integration/, run via scripts/test-integration.sh) to exercise the git
# handler's real SSH transport and host-key verification, which the fully
# in-process unit tests in internal/handlers/git can't reach.
FROM alpine:3.20

RUN apk add --no-cache openssh git bash netcat-openbsd && \
    ssh-keygen -A && \
    adduser -D -s /bin/bash gituser && \
    passwd -u gituser && \
    mkdir -p /home/gituser/.ssh /srv/git && \
    chown gituser:gituser /home/gituser/.ssh /srv/git && \
    chmod 700 /home/gituser/.ssh

# authorized_keys is generated fresh by scripts/test-integration.sh before the image is
# built - see test/integration/testdata/, which is gitignored, not committed.
COPY authorized_keys /home/gituser/.ssh/authorized_keys
RUN chown gituser:gituser /home/gituser/.ssh/authorized_keys && \
    chmod 600 /home/gituser/.ssh/authorized_keys

COPY sshd_config /etc/ssh/sshd_config
COPY init-fixture-repo.sh /usr/local/bin/init-fixture-repo.sh
RUN chmod +x /usr/local/bin/init-fixture-repo.sh && \
    su gituser -c /usr/local/bin/init-fixture-repo.sh

EXPOSE 22
CMD ["/usr/sbin/sshd", "-D", "-e"]
