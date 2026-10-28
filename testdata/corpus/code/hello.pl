use strict;
use warnings;

sub answer {
    my ($n) = @_;
    return $n * 2;
}

my $total = answer(21);
print "total $total\n";
