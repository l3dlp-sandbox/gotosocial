# Managing GtS on slow hardware

While GoToSocial runs great on lower-end hardware, some operations are not practical on it, especially
instances with the database on slow storage (think anything that is not an SSD).  This document
offers some suggestions on how to work around common issues when running GtS on slow hardware.

## Running database migrations on a different machine

Sometimes a database migration will need to do operations that are taxing on the database's storage.
These operations can take days if the database resides on a hard disk or SD card. If your
database is on slow storage, it can save a lot of time to follow the following procedure:

!!! danger

    It might seem tempting to keep GtS running while you run the migrations on another machine, but
    doing this will lead to all the posts that are received during the migration post disappearing
    once the migrated database is re-imported.

 1. Shut down GtS
 2. Take a [backup](backup_and_restore.md#what-to-backup-database) of the database
 3. Import the database on faster hardware
 4. Run the GtS migration on the faster hardware
 5. Take a backup of the resultant database
 6. Import the resultant backup and overwrite the old database
 7. Start GtS with the new version

### Running GtS migrations separately

After you import the database on the faster hardware, you can run the migration without starting
GtS by downloading the *target* GtS version from the [releases](https://codeberg.org/superseriousbusiness/gotosocial/releases) page.
For instance, if you are running `v0.19.2` and you want to upgrade to `v0.20.0-rc1`, download the
latter version. Once you have the binary, set it to executable by running `chmod u+x /path/to/gotosocial`. Afterwards, copy the configuration of the original server, and alter
it with the location of the new database. We copy the configuration in case variables like
the hostname is used in the migration, we want to keep that consistent.
Once everything is in place, you can run the migration like this:

```sh
$ /path/to/gotosocial --config-path /path/to/config migrations run
```

This will run all the migrations, just like GtS would if it was started normally. Once this is done
you can copy the result to the original instance and start the new GtS version there as well, which
will see that everything is migrated and that there's nothing to do except run as expected.
